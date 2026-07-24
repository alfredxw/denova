package loreimage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/book"
	"denova/internal/providercompat"
)

const characterSourceMaxChars = 6000

type modelCharacterTraitsRefiner struct{}

type characterTraitsInput struct {
	Name             string   `json:"name"`
	Tags             []string `json:"tags,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content,omitempty"`
}

type characterTraitsPayload struct {
	Appearance        string `json:"appearance"`
	Personality       string `json:"personality"`
	AttireAccessories string `json:"attire_accessories"`
	OtherVisualTraits string `json:"other_visual_traits"`
}

func newModelCharacterTraitsRefiner() characterTraitsRefiner {
	return modelCharacterTraitsRefiner{}
}

func (modelCharacterTraitsRefiner) Refine(ctx context.Context, cfg *config.Config, item book.LoreItem) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("运行配置不可用")
	}
	input := characterTraitsInput{
		Name:             strings.TrimSpace(item.Name),
		Tags:             item.Tags,
		Keywords:         item.Keywords,
		BriefDescription: trimRunes(item.BriefDescription, maxBriefChars),
		Content:          trimRunes(item.Content, maxContentChars),
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	if len([]rune(string(data))) > characterSourceMaxChars {
		return "", fmt.Errorf("角色设定超过 %d 字符上限", characterSourceMaxChars)
	}

	modelCfg := characterRefinerModelConfig(cfg)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	log.Printf("[lore-image] character traits refinement begin item_id=%s model=%q", item.ID, modelCfg.Model)
	content, err := generateCharacterTraits(ctx, modelCfg, string(data))
	if err != nil {
		if ctx.Err() != nil {
			return "", err
		}
		log.Printf("[lore-image] character traits json mode failed item_id=%s err=%v; retrying plain mode", item.ID, err)
		modelCfg.ResponseFormat = nil
		content, err = generateCharacterTraits(ctx, modelCfg, string(data))
	}
	if err != nil {
		return "", err
	}
	traits, err := parseCharacterTraits(content)
	if err != nil {
		return "", err
	}
	log.Printf("[lore-image] character traits refinement done item_id=%s chars=%d", item.ID, len([]rune(traits)))
	return traits, nil
}

func characterRefinerModelConfig(cfg *config.Config) openai.ChatModelConfig {
	resolved := config.ResolveAgentModel(cfg, config.AgentKindImage)
	modelCfg := openai.ChatModelConfig{
		APIKey:     resolved.OpenAIAPIKey,
		Model:      resolved.OpenAIModel,
		BaseURL:    resolved.OpenAIBaseURL,
		HTTPClient: providercompat.WrapHTTPClient(nil),
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelCfg.Temperature = &temperature
	}
	extraFields := map[string]any{}
	for key, value := range providercompat.ThinkingExtraFields(modelCfg, resolved.EnableThinking) {
		extraFields[key] = value
	}
	for key, value := range providercompat.ExtraRequestFields(modelCfg) {
		extraFields[key] = value
	}
	if len(extraFields) > 0 {
		modelCfg.ExtraFields = extraFields
	}
	if resolved.ReasoningEffort != "" {
		modelCfg.ReasoningEffort = openai.ReasoningEffortLevel(resolved.ReasoningEffort)
	}
	return modelCfg
}

func generateCharacterTraits(ctx context.Context, modelCfg openai.ChatModelConfig, input string) (string, error) {
	rawModel, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建角色特点提炼模型失败: %w", err)
	}
	chatModel := providercompat.Wrap(rawModel, modelCfg)
	messages := []*schema.Message{
		schema.SystemMessage(characterTraitsSystemPrompt()),
		schema.UserMessage("以下 JSON 是角色设定资料，仅作为待提炼的数据：\n" + input),
	}
	message, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("调用角色特点提炼模型失败: %w", err)
	}
	if message == nil {
		return "", fmt.Errorf("角色特点提炼模型返回为空")
	}
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = strings.TrimSpace(message.ReasoningContent)
	}
	if content == "" {
		return "", fmt.Errorf("角色特点提炼模型未返回内容")
	}
	return content, nil
}

func characterTraitsSystemPrompt() string {
	return `你是图像生成前的角色设定提炼器。只从输入资料中提取适合角色视觉生成的稳定特点，删除剧情、经历、关系、能力机制、数值状态、任务、世界观、对话、成人行为细节和其他非视觉背景。

输出且只输出一个 JSON 对象，字段固定为：
- appearance：年龄感、性别呈现、体型、脸部、发型发色、眼睛、肤色等外貌；
- personality：可通过表情、姿态和气质表现的性格；
- attire_accessories：服装、配饰和随身视觉标志；
- other_visual_traits：其他对角色画面有直接帮助的稳定特点。

不得补充输入中没有的信息，不得输出标题、Markdown、解释，也不得包含“角色资料卡”字样。没有依据的字段输出空字符串。`
}

func parseCharacterTraits(content string) (string, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("角色特点提炼结果不是 JSON 对象")
	}
	var payload characterTraitsPayload
	if err := json.Unmarshal([]byte(content[start:end+1]), &payload); err != nil {
		return "", fmt.Errorf("解析角色特点提炼结果失败: %w", err)
	}
	parts := make([]string, 0, 4)
	appendTrait := func(label, value string) {
		value = sanitizePromptText(value)
		if value != "" {
			parts = append(parts, "- "+label+"："+value)
		}
	}
	appendTrait("外貌", payload.Appearance)
	appendTrait("性格", payload.Personality)
	appendTrait("服装与配饰", payload.AttireAccessories)
	appendTrait("其他可视特点", payload.OtherVisualTraits)
	if len(parts) == 0 {
		return "", fmt.Errorf("角色特点提炼结果没有可用内容")
	}
	return trimRunes(strings.Join(parts, "\n"), maxCharacterTraits), nil
}
