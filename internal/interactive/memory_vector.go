package interactive

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MemoryEmbedder 是叙事记忆的向量来源。interactive 包只依赖这个窄接口,
// 具体的 HTTP 客户端由上层注入,保证检索层可以在无网络的测试里被完整覆盖。
type MemoryEmbedder interface {
	// EmbedMemoryTexts 批量取向量,返回顺序与入参一一对应。
	EmbedMemoryTexts(ctx context.Context, texts []string) ([][]float32, error)
	// EmbeddingModelID 标识向量的产出模型。换模型后旧向量的语义空间不再可比,
	// 缓存层据此整批失效而非静默混用。
	EmbeddingModelID() string
}

// memoryVectorEntry 是向量侧车文件的一行。侧车与事件日志同为 append-only,
// 但它是纯缓存:整个文件删掉只会让检索退回关键词路径,不丢任何事实。
// 因此它不进事件链,也不参与分支/epoch 语义。
type memoryVectorEntry struct {
	RecordID string `json:"record_id"`
	Model    string `json:"model"`
	Dim      int    `json:"dim"`
	// Vector 是 float32 小端字节的 base64。相比 JSON 数字数组可省约六成体积,
	// 解析也不必逐个数字走反射。
	Vector string `json:"vector"`
}

func (s *Store) memoryVectorPath(storyID string) string {
	return filepath.Join(s.storyDir(), "story-"+storyID+".vectors.jsonl")
}

// MemoryVectorText 组装一条记录被嵌入的文本。检索查询侧必须用可比的措辞组装,
// 否则向量空间里的距离没有意义。
func MemoryVectorText(record NarrativeMemoryRecord) string {
	parts := make([]string, 0, 3)
	if subject := strings.TrimSpace(record.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if object := strings.TrimSpace(record.Object); object != "" {
		parts = append(parts, object)
	}
	if text := strings.TrimSpace(record.Text); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, " | ")
}

// readMemoryVectorsLocked 读出侧车中属于当前 embedding 模型的向量。
// 同一 record 多次出现时后写的胜出(重新抽取会覆盖旧向量)。
func (s *Store) readMemoryVectorsLocked(storyID, model string) map[string][]float32 {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	file, err := os.Open(s.memoryVectorPath(storyID))
	if err != nil {
		// 侧车缺失是正常状态(从未抽取过向量),不是错误。
		return nil
	}
	defer file.Close()

	vectors := map[string][]float32{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxMemoryVectorLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry memoryVectorEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.RecordID == "" || entry.Model != model {
			continue
		}
		vector, err := decodeMemoryVector(entry.Vector, entry.Dim)
		if err != nil {
			continue
		}
		vectors[entry.RecordID] = vector
	}
	return vectors
}

// maxMemoryVectorLine 上限按 4096 维 float32 的 base64 体积留足余量。
const maxMemoryVectorLine = 64 * 1024

// AppendMemoryVectors 把记录向量追加进侧车。已存在的 record 会被新行覆盖
// (读取时后写胜出),因此重复调用是幂等的。
func (s *Store) AppendMemoryVectors(storyID, model string, vectors map[string][]float32) error {
	model = strings.TrimSpace(model)
	if model == "" || len(vectors) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.storyDir(), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.memoryVectorPath(storyID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	// map 迭代顺序不定;侧车按 record ID 排序写入,让文件在多次运行间可比对。
	for _, recordID := range sortedKeys(vectors) {
		vector := vectors[recordID]
		if len(vector) == 0 {
			continue
		}
		data, err := json.Marshal(memoryVectorEntry{
			RecordID: recordID,
			Model:    model,
			Dim:      len(vector),
			Vector:   encodeMemoryVector(vector),
		})
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func encodeMemoryVector(vector []float32) string {
	buf := make([]byte, 4*len(vector))
	for i, value := range vector {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(value))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func decodeMemoryVector(encoded string, dim int) ([]float32, error) {
	buf, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("向量字节数 %d 不是 4 的倍数", len(buf))
	}
	vector := make([]float32, len(buf)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	if dim > 0 && len(vector) != dim {
		return nil, fmt.Errorf("向量维度不符: 声明 %d,实际 %d", dim, len(vector))
	}
	return vector, nil
}

// cosineSimilarity 返回 [-1,1];任一向量为零向量或维度不符时返回 0,
// 让该候选自然落到向量排名之外而不是污染排序。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func sortedKeys(m map[string][]float32) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
