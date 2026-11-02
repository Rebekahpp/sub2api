package kiro

import (
	"strings"
)

// ExtractThinkingFromContent separates a leading inline reasoning envelope
// from assistantResponseEvent answer text. Native reasoning events are separate.
func ExtractThinkingFromContent(content string) (visible string, thinking string) {
	var redactor InlineThinkingRedactor
	visible, thinking = redactor.Push(content)
	tailVisible, tailThinking := redactor.Flush()
	if tailVisible != "" {
		if visible == "" {
			visible = tailVisible
		} else {
			visible += tailVisible
		}
	}
	if tailThinking != "" {
		if thinking == "" {
			thinking = tailThinking
		} else {
			thinking += tailThinking
		}
	}
	return strings.TrimSpace(visible), strings.TrimSpace(thinking)
}

// InlineThinkingRedactor recognizes one leading <thinking>...</thinking>
// envelope (after optional ASCII whitespace), the legacy Kiro framing also used
// by thinking_stash fixtures. Once answer text starts, every byte is literal:
// a later tag, including one in Markdown/code, cannot open another envelope.
// This is a framing rule, not a guess based on the words in the response.
//
// A bare literal starting with <thinking> is indistinguishable from that legacy
// envelope. Keep the conservative interpretation there, including truncated
// envelopes, rather than expose genuine reasoning. Native reasoning events do
// not use this parser.
type InlineThinkingRedactor struct {
	pending     string
	textPending string
	inThinking  bool
	bodyStarted bool
}

var kiroSelfIDReplacements = []struct {
	from string
	to   string
}{
	{from: "我是 Kiro", to: "我是 Claude"},
	{from: "我是Kiro", to: "我是 Claude"},
	{from: "我叫 Kiro", to: "我叫 Claude"},
	{from: "我叫Kiro", to: "我叫 Claude"},
	{from: "以 Kiro 的身份", to: "以 Claude 的身份"},
	{from: "作为 Kiro", to: "作为 Claude"},
	{from: "作为Kiro", to: "作为 Claude"},
	{from: "由亚马逊云科技（AWS）", to: "由 Anthropic "},
	{from: "由亚马逊云科技 (AWS) ", to: "由 Anthropic "},
	{from: "由亚马逊云科技 (AWS)", to: "由 Anthropic"},
	{from: "I am Kiro", to: "I am Claude"},
	{from: "I'm Kiro", to: "I'm Claude"},
	{from: "myself as Kiro", to: "myself as Claude"},
	{from: "identify as Kiro", to: "identify as Claude"},
	{from: "acting as Kiro", to: "acting as Claude"},
	{from: "running as Kiro", to: "running as Claude"},
}

// Push preserves framing across chunks, holding only a possible opening prefix
// before the answer starts or a possible closing suffix inside reasoning.
func (r *InlineThinkingRedactor) Push(content string) (visible string, thinking string) {
	r.pending += content
	var out strings.Builder
	var reasoning strings.Builder

	for r.pending != "" {
		if r.inThinking {
			end := strings.Index(r.pending, "</thinking>")
			if end == -1 {
				keep := longestTagPrefixSuffix(r.pending, "</thinking>")
				if len(r.pending) > keep {
					_, _ = reasoning.WriteString(r.pending[:len(r.pending)-keep])
					r.pending = r.pending[len(r.pending)-keep:]
				}
				break
			}
			_, _ = reasoning.WriteString(r.pending[:end])
			r.pending = r.pending[end+len("</thinking>"):]
			r.inThinking = false
			r.bodyStarted = true
			continue
		}

		if r.bodyStarted {
			_, _ = out.WriteString(r.pending)
			r.pending = ""
			break
		}

		// Leading whitespace is not evidence that the answer has started.
		trimmed := strings.TrimLeft(r.pending, " \t\r\n")
		_, _ = out.WriteString(r.pending[:len(r.pending)-len(trimmed)])
		r.pending = trimmed
		if r.pending == "" {
			break
		}
		if strings.HasPrefix(r.pending, "<thinking>") {
			r.pending = r.pending[len("<thinking>"):]
			r.inThinking = true
			continue
		}
		if strings.HasPrefix("<thinking>", r.pending) {
			break // opening delimiter may continue in the next chunk
		}
		r.bodyStarted = true
	}

	r.textPending += out.String()
	return r.drainVisibleText(false), reasoning.String()
}

// Flush releases any text held for split-tag detection. If upstream ends while a
// thinking block is still open, the buffered tail is treated as reasoning so it
// remains redacted rather than leaking into visible text.
func (r *InlineThinkingRedactor) Flush() (visible string, thinking string) {
	if r.pending != "" {
		if r.inThinking {
			thinking = r.pending
		} else if !r.bodyStarted && strings.HasPrefix("<thinking>", r.pending) {
			thinking = r.pending
		} else {
			r.textPending += r.pending
		}
		r.pending = ""
		r.inThinking = false
		r.bodyStarted = true
	}
	visible = r.drainVisibleText(true)
	return visible, thinking
}

func (r *InlineThinkingRedactor) drainVisibleText(flush bool) string {
	if r.textPending == "" {
		return ""
	}
	r.textPending = redactKiroSelfIDText(r.textPending)
	if flush {
		out := r.textPending
		r.textPending = ""
		return out
	}
	keep := longestSelfIDPrefixSuffix(r.textPending)
	if keep >= len(r.textPending) {
		return ""
	}
	out := r.textPending[:len(r.textPending)-keep]
	r.textPending = r.textPending[len(r.textPending)-keep:]
	return out
}

func redactKiroSelfIDText(s string) string {
	for _, rule := range kiroSelfIDReplacements {
		if strings.Contains(s, rule.from) {
			s = strings.ReplaceAll(s, rule.from, rule.to)
		}
	}
	return s
}

func longestSelfIDPrefixSuffix(s string) int {
	best := 0
	for _, rule := range kiroSelfIDReplacements {
		if keep := longestTagPrefixSuffix(s, rule.from); keep > best {
			best = keep
		}
	}
	return best
}

func longestTagPrefixSuffix(s, tag string) int {
	max := len(tag) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}
