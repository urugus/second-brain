package claude

import (
	"fmt"
	"strings"

	"github.com/urugus/second-brain/internal/model"
)

const systemPrompt = `You are a knowledge consolidation agent for a personal "Second Brain" system.
Analyze a completed work session and produce structured knowledge updates.

You will receive session metadata, chronological events, notes, tasks, and a list of existing knowledge base files.

Your job:
1. Produce a concise summary of what was accomplished.
2. Identify knowledge worth preserving long-term and produce KB file updates.
   - For existing files: return the complete updated content (not a diff).
   - For new topics: create new files with descriptive filenames.
   - File paths should use lowercase-kebab-case with .md extension.
   - Organize into subdirectories by topic if appropriate.
   - Write in clear, concise Markdown.
3. Suggest actionable follow-up tasks.

Guidelines:
- Only create/update KB files for genuinely useful long-term knowledge.
- Do NOT create entries for trivial or ephemeral information.
- Merge new information into existing files when the topic already exists.
- If there is nothing worth persisting, return empty kb_updates.`

const jsonSchema = `{
  "type": "object",
  "required": ["summary", "kb_updates", "suggested_tasks"],
  "properties": {
    "summary": {
      "type": "string",
      "description": "Concise summary of the session"
    },
    "kb_updates": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["path", "content", "reason"],
        "properties": {
          "path": {
            "type": "string",
            "description": "Relative file path (e.g. 'golang/error-handling.md')"
          },
          "content": {
            "type": "string",
            "description": "Complete Markdown content for this file"
          },
          "reason": {
            "type": "string",
            "description": "Brief explanation of why this file is being created/updated"
          }
        }
      }
    },
    "suggested_tasks": {
      "type": "array",
      "items": {
        "type": "string",
        "description": "Actionable follow-up task"
      }
    }
  }
}`

func buildConsolidationPrompt(session *model.Session, events []model.Event, notes []model.Note, tasks []model.Task, kbFiles []string) string {
	var b strings.Builder

	b.WriteString(systemPrompt)
	b.WriteString("\n\n---\n\n")

	// Session metadata
	b.WriteString("## Session\n")
	fmt.Fprintf(&b, "- Title: %s\n", session.Title)
	fmt.Fprintf(&b, "- Goal: %s\n", session.Goal)
	fmt.Fprintf(&b, "- Status: %s\n", session.Status)
	fmt.Fprintf(&b, "- Started: %s\n", session.StartedAt.Format("2006-01-02 15:04"))
	if session.EndedAt != nil {
		fmt.Fprintf(&b, "- Ended: %s\n", session.EndedAt.Format("2006-01-02 15:04"))
	}
	if session.Summary != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", session.Summary)
	}

	// Events
	if len(events) > 0 {
		b.WriteString("\n## Events (chronological)\n")
		for _, e := range events {
			fmt.Fprintf(&b, "[%s] %s: %s\n", e.CreatedAt.Format("15:04:05"), e.Type, e.Payload)
		}
	}

	// Notes
	if len(notes) > 0 {
		b.WriteString("\n## Notes\n")
		for _, n := range notes {
			tags := ""
			if len(n.Tags) > 0 {
				tags = fmt.Sprintf(" [tags: %s]", strings.Join(n.Tags, ", "))
			}
			fmt.Fprintf(&b, "- %s%s\n", n.Content, tags)
		}
	}

	// Tasks
	if len(tasks) > 0 {
		b.WriteString("\n## Tasks\n")
		for _, t := range tasks {
			fmt.Fprintf(&b, "- [%s] %s", t.Status, t.Title)
			if t.Description != "" {
				fmt.Fprintf(&b, ": %s", t.Description)
			}
			b.WriteString("\n")
		}
	}

	// Existing KB
	if len(kbFiles) > 0 {
		b.WriteString("\n## Existing Knowledge Base Files\n")
		for _, f := range kbFiles {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	return b.String()
}

const sleepSystemPrompt = `You are a knowledge consolidation agent for a personal "Second Brain" system.
You are performing "sleep-mode" consolidation: processing accumulated notes that were
collected over time from various sources (Slack messages, observations, etc.).

Unlike session-based consolidation, there is no specific work session context. These notes
were gathered incrementally by an automated sync process and need to be organized into
the long-term knowledge base.

You will receive a list of unconsolidated notes and existing knowledge base files.

Your job:
1. Replay and abstract the note set:
   - Group near-duplicate notes into one abstracted point.
   - Preserve only differences that add meaningful knowledge.
2. Produce a concise summary of what themes and information the notes contain.
3. Identify knowledge worth preserving long-term and produce KB file updates.
   - For existing files: return the complete updated content (not a diff).
   - For new topics: create new files with descriptive filenames.
   - File paths should use lowercase-kebab-case with .md extension.
   - Organize into subdirectories by topic if appropriate.
   - Write in clear, concise Markdown.
4. Suggest actionable follow-up tasks if any notes imply action is needed.

Guidelines:
- Only create/update KB files for genuinely useful long-term knowledge.
- Do NOT create entries for trivial or ephemeral information.
- Merge new information into existing files when the topic already exists.
- Group related notes together when updating/creating KB files.
- Avoid duplicate KB updates for the same path; return at most one final update per path.
- If there is nothing worth persisting, return empty kb_updates.
- Notes may come from different time periods -- focus on content, not chronology.`

func buildSleepConsolidationPrompt(notes []model.Note, kbFiles []string) string {
	var b strings.Builder

	b.WriteString(sleepSystemPrompt)
	b.WriteString("\n\n---\n\n")

	if len(notes) > 0 {
		b.WriteString("## Unconsolidated Notes\n")
		for _, n := range notes {
			tags := ""
			if len(n.Tags) > 0 {
				tags = fmt.Sprintf(" [tags: %s]", strings.Join(n.Tags, ", "))
			}
			source := ""
			if n.Source != "" {
				source = fmt.Sprintf(" (source: %s)", n.Source)
			}
			fmt.Fprintf(&b, "- [%s] %s%s%s\n",
				n.CreatedAt.Format("2006-01-02 15:04"), n.Content, tags, source)
		}
	}

	if len(kbFiles) > 0 {
		b.WriteString("\n## Existing Knowledge Base Files\n")
		for _, f := range kbFiles {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	return b.String()
}

func buildSummarizePrompt(text string) string {
	return fmt.Sprintf("Summarize the following text in 2-3 sentences:\n\n%s", text)
}
