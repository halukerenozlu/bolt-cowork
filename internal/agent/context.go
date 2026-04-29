package agent

import (
	"fmt"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

const (
	MaxContextMessages = 20
	MaxContextChars    = 32000
)

// trimHistory returns a trimmed copy of messages safe to send to the LLM.
//
// Algorithm:
//  1. If the first message is RoleSystem, it is always preserved.
//  2. If remaining messages exceed MaxContextMessages, the oldest are dropped.
//  3. If total character count (system included) exceeds MaxContextChars, the
//     oldest non-system messages are dropped one at a time.
//  4. When any messages are dropped, a RoleUser summary is inserted immediately
//     after the system message (if any) to inform the model.
func trimHistory(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}

	// Separate optional system message.
	var system *types.Message
	rest := messages
	if messages[0].Role == types.RoleSystem {
		sys := messages[0]
		system = &sys
		rest = messages[1:]
	}

	omitted := 0

	// Trim by message count.
	if len(rest) > MaxContextMessages {
		omitted += len(rest) - MaxContextMessages
		rest = rest[len(rest)-MaxContextMessages:]
	}

	// Trim by character count.
	for len(rest) > 0 {
		total := 0
		if system != nil {
			total += len(system.Content)
		}
		for _, m := range rest {
			total += len(m.Content)
		}
		if total <= MaxContextChars {
			break
		}
		rest = rest[1:]
		omitted++
	}

	// If a summary will be inserted, reserve a slot for it (and for the system
	// message if present) so the total output stays within MaxContextMessages.
	if omitted > 0 {
		reserved := 1 // summary slot
		if system != nil {
			reserved++ // system slot
		}
		maxRest := MaxContextMessages - reserved
		if maxRest < 0 {
			maxRest = 0
		}
		if len(rest) > maxRest {
			omitted += len(rest) - maxRest
			rest = rest[len(rest)-maxRest:]
		}
	}

	// Build output slice.
	var result []types.Message
	if system != nil {
		result = append(result, *system)
	}
	if omitted > 0 {
		result = append(result, types.Message{
			Role:    types.RoleUser,
			Content: fmt.Sprintf("[Earlier conversation trimmed. %d messages omitted.]", omitted),
		})
	}
	result = append(result, rest...)
	return result
}
