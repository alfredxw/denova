package conversationjournal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type transactionBody struct {
	Journal              string            `json:"journal"`
	Version              int               `json:"version"`
	Identity             Identity          `json:"identity"`
	Cursor               Cursor            `json:"cursor"`
	PreviousRecordSHA256 string            `json:"previous_record_sha256,omitempty"`
	CommittedAt          time.Time         `json:"committed_at"`
	Records              []json.RawMessage `json:"records"`
}

type transaction struct {
	transactionBody
	Checksum string `json:"checksum"`
}

func encodeTransaction(identity Identity, cursor Cursor, previousSHA string, payloads []json.RawMessage) ([]byte, error) {
	body := transactionBody{
		Journal: transactionKind, Version: transactionVersion, Identity: identity,
		Cursor: cursor, PreviousRecordSHA256: previousSHA, CommittedAt: time.Now().UTC(),
		Records: clonePayloads(payloads),
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode conversation transaction checksum body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	encoded, err := json.Marshal(transaction{transactionBody: body, Checksum: hex.EncodeToString(digest[:])})
	if err != nil {
		return nil, fmt.Errorf("encode conversation transaction: %w", err)
	}
	return encoded, nil
}

func decodeTransaction(line []byte) (transactionBody, bool, error) {
	var discriminator struct {
		Journal string `json:"journal"`
	}
	if err := json.Unmarshal(line, &discriminator); err != nil {
		return transactionBody{}, false, err
	}
	if discriminator.Journal != transactionKind {
		return transactionBody{}, false, nil
	}
	var decoded transaction
	if err := json.Unmarshal(line, &decoded); err != nil {
		return transactionBody{}, true, err
	}
	if decoded.Version != transactionVersion {
		return transactionBody{}, true, fmt.Errorf("unsupported conversation transaction version %d", decoded.Version)
	}
	bodyJSON, err := json.Marshal(decoded.transactionBody)
	if err != nil {
		return transactionBody{}, true, err
	}
	digest := sha256.Sum256(bodyJSON)
	if decoded.Checksum != hex.EncodeToString(digest[:]) {
		return transactionBody{}, true, fmt.Errorf("conversation transaction checksum mismatch")
	}
	if decoded.Cursor == 0 || len(decoded.Records) == 0 {
		return transactionBody{}, true, fmt.Errorf("conversation transaction cursor and records are required")
	}
	for index, payload := range decoded.Records {
		if !json.Valid(payload) {
			return transactionBody{}, true, fmt.Errorf("conversation transaction record %d is invalid JSON", index+1)
		}
	}
	return decoded.transactionBody, true, nil
}

func trimRecord(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	return line
}

func recordSHA256(line []byte) string {
	digest := sha256.Sum256(line)
	return hex.EncodeToString(digest[:])
}

func clonePayloads(payloads []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(payloads))
	for index, payload := range payloads {
		result[index] = append(json.RawMessage(nil), payload...)
	}
	return result
}
