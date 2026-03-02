package sync

import (
	"fmt"
	"strings"
)

func buildSyncPrompt(profile *focusProfile) string {
	var b strings.Builder
	b.WriteString(`You are a sync agent for a personal "Second Brain" knowledge management system.
Your job is to collect important information from all available sources and save what is worth keeping.

## Instructions

1. **Discover available tools**: Check all MCP tools available to you.
   Use broad source coverage, but do NOT process every message equally.
   Prioritize likely user-relevant information first.

2. **Apply relevance ranking while collecting**:
   - Focus first on items tied to the user's active projects, tasks, and recurring topics
   - In high-volume sources (for example, Slack), prioritize channels/threads/messages
     that match the learned relevance profile below
   - De-prioritize repetitive chatter, broad announcements, and low-signal noise
`)
	if profile != nil && !profile.isEmpty() {
		fmt.Fprintf(&b, `
## Learned relevance profile (adaptive)
This profile is inferred from previously saved notes and tasks.
Treat it as ranking guidance (soft preference), not a strict filter.

- Priority tags: %s
- Priority keywords: %s
- Active task keywords: %s
`,
			commaOrNone(profile.Tags),
			commaOrNone(profile.Terms),
			commaOrNone(profile.TaskTerms),
		)
	} else {
		b.WriteString(`
## Learned relevance profile (adaptive)
No stable profile yet. Start broad, then let saved notes/tasks define future focus.
`)
	}

	b.WriteString(`
3. **Check existing second-brain data** to avoid duplicates:
   - Use list_notes to see recent notes
   - Use list_tasks to see current tasks
   - Use kb_list and kb_search to check existing knowledge

4. **Save what matters** using second-brain MCP tools:
   - Use create_note for important information (include tags and source)
   - Use create_task for actionable items (include description and priority)
   - Use kb_write to update knowledge base files if information has changed

5. **Guidelines**:
   - Only save genuinely important or actionable information
   - Do NOT save trivial messages, routine status updates, or noise
   - Tag notes with their source (e.g. the tool or service name)
   - For tasks, set appropriate priority (1=low, 2=medium, 3=high)
   - Merge new information into existing KB files when the topic already exists
   - If nothing important is found, that is fine -- report an empty sync

6. **Return a summary** of what you did.`)

	return b.String()
}

func commaOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

const syncJSONSchema = `{
  "type": "object",
  "required": ["summary", "notes_added", "tasks_added", "kb_files_updated"],
  "properties": {
    "summary": {
      "type": "string",
      "description": "Human-readable summary of what was synced"
    },
    "notes_added": {
      "type": "integer",
      "description": "Number of notes created"
    },
    "tasks_added": {
      "type": "integer",
      "description": "Number of tasks created"
    },
    "kb_files_updated": {
      "type": "array",
      "items": { "type": "string" },
      "description": "List of KB file paths that were written or updated"
    }
  }
}`
