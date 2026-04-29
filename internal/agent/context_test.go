package agent

import (
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

func TestTrimHistory_UnderLimit(t *testing.T) {
	msgs := make([]types.Message, 10)
	for i := range msgs {
		msgs[i] = types.Message{Role: types.RoleUser, Content: "msg"}
	}
	result := trimHistory(msgs)
	if len(result) != 10 {
		t.Errorf("expected 10 messages unchanged, got %d", len(result))
	}
}

func TestTrimHistory_OverMessageLimit(t *testing.T) {
	msgs := make([]types.Message, 25)
	for i := range msgs {
		msgs[i] = types.Message{Role: types.RoleUser, Content: "msg"}
	}
	result := trimHistory(msgs)
	// No system: summary takes 1 slot → rest = MaxContextMessages-1 = 19, total = 20.
	// omitted = 25 - 19 = 6.
	if len(result) != MaxContextMessages {
		t.Errorf("expected %d messages (summary + 19 rest), got %d", MaxContextMessages, len(result))
	}
	if result[0].Role != types.RoleUser || !strings.Contains(result[0].Content, "6 messages omitted") {
		t.Errorf("expected summary at index 0 with '6 messages omitted', got: %q", result[0].Content)
	}
}

func TestTrimHistory_OverCharLimit(t *testing.T) {
	// 3 messages of ~15 000 chars each → total ~45 000 > MaxContextChars (32 000)
	bigContent := strings.Repeat("x", 15000)
	msgs := []types.Message{
		{Role: types.RoleUser, Content: bigContent},
		{Role: types.RoleAssistant, Content: bigContent},
		{Role: types.RoleUser, Content: bigContent},
	}
	result := trimHistory(msgs)

	found := false
	for _, m := range result {
		if strings.Contains(m.Content, "messages omitted") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a summary message indicating messages were trimmed for char limit")
	}
}

func TestTrimHistory_SystemPreserved(t *testing.T) {
	sysMsg := types.Message{Role: types.RoleSystem, Content: "system instructions"}
	msgs := []types.Message{sysMsg}
	for i := 0; i < 25; i++ {
		msgs = append(msgs, types.Message{Role: types.RoleUser, Content: "msg"})
	}
	result := trimHistory(msgs)

	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	if result[0].Role != types.RoleSystem {
		t.Errorf("first message role = %q, want %q", result[0].Role, types.RoleSystem)
	}
	if result[0].Content != "system instructions" {
		t.Errorf("system message content changed to %q", result[0].Content)
	}
}
