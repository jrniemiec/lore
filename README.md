# lore

> Terminal-first AI chat in Go. Simple, composable, fast.

`lore` is a keyboard-driven TUI chat application for daily LLM work in the terminal. It supports persistent multi-topic conversations, multiple provider profiles, context management strategies, file injection via `@ref` syntax, text-to-speech playback, and a full headless CLI mode for scripting and automation.

---

## Contents

- [Features](#features)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [TUI interface](#tui-interface)
- [Key bindings](#key-bindings)
- [Slash commands](#slash-commands)
- [File injection (@ref)](#file-injection-ref)
- [Resources](#resources)
- [Headless mode](#headless-mode)
- [Context strategies](#context-strategies)
- [Text-to-speech](#text-to-speech)
- [Providers](#providers)
- [Data layout](#data-layout)

---

## Features

- **Full TUI** — Bubbletea-based, keyboard-driven, streaming tokens displayed live
- **Persistent topics** — each conversation is a named topic with its own history, system prompt, and attached files
- **Multiple profiles** — switch between providers and models within a session
- **Three context strategies** — tail, token-budget, summarize (rolling LLM-generated summary)
- **File injection** — embed file content into any prompt with `@name`, `@./path`, `@~/path`, or `@/abs/path`; multiple refs per prompt
- **Resources** — per-topic file library; managed with `/resource-add`, `/resource-list`, `/resource-remove`
- **Exchange navigation** — `Ctrl+N` into the conversation, browse with arrows, expand/collapse, delete, speak
- **Text-to-speech** — `s` speaks any exchange; `/play-all` queues the whole conversation; `/tts on` auto-speaks every response
- **Model override** — `-m <model>` at startup overrides the model within the active profile without creating a new profile entry
- **Headless mode** — full CLI for scripting: pipe stdin, read from files, all admin ops as flags
- **Personal notes** — `// text` saves a note to history that is never sent to the LLM
- **Input history** — bash-style Up/Down browsing (in-memory, max 128 entries)
- **Command completion** — type `/` to see completions; Tab fills selection into input, Enter executes it
- **Single binary** — no runtime dependencies; providers and TTS are optional

---

## Requirements

**Terminal:** The TUI requires [iTerm2](https://iterm2.com). Both dark and light background profiles are supported. Other terminals (macOS Terminal.app, VS Code terminal, Emacs shell, vterm) are not supported for TUI mode. Use `--nw` (headless) in those environments.

**API keys:** Set in your environment before running:
- Anthropic: `ANTHROPIC_API_KEY` (or `LORE_ANTHROPIC_API_KEY`)
- OpenAI: `OPENAI_API_KEY` (or `LORE_OPENAI_API_KEY`)
- Ollama: no key needed — set `LORE_OLLAMA_HOST` if not on localhost

---

## Installation

**Go:** 1.24 or later required to build from source.

```bash
git clone <repo-url>
cd lore
make install        # runs tests, builds, copies to ~/dev/bin/lore
```

Or build only:

```bash
make build          # outputs to ./bin/lore
```

The binary is self-contained. No external runtime is required for the core functionality.

---

## Quick start

```bash
lore                               # open TUI on default topic
lore -t myproject                  # open TUI on topic "myproject"
lore -t myproject -p sonnet        # open TUI on topic + named profile
lore -t myproject -m claude-opus-4-6  # open TUI with model override
lore -p gpt4 "explain X"          # headless: one-shot with a specific profile
echo "summarize this" | lore       # headless: piped prompt
```

On first run, `lore` bootstraps `~/.lore/config.json` from `~/.ask/config.json` if it exists, rewriting `topics_root` to `~/.lore/topics/`. If neither exists, an empty config is created — add at least one profile to get started (see [Configuration](#configuration)).

---

## Configuration

Config lives at `~/.lore/config.json` (override with `LORE_HOME=/path/to/dir`).

```json
{
  "topics_root": "~/.lore/topics",
  "default_topic": "dev",
  "default_profile": "haiku",
  "window_messages": 1024,
  "profiles": {
    "haiku": {
      "provider": "anthropic",
      "model": "claude-haiku-4-5-20251001",
      "max_context_tokens": 200000,
      "info": { "input_cost_per_1m": 0.80, "output_cost_per_1m": 4.00 }
    },
    "sonnet": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-6",
      "max_context_tokens": 200000,
      "strategy": "summarize",
      "summarizer_profile": "haiku",
      "info": { "input_cost_per_1m": 3.00, "output_cost_per_1m": 15.00 }
    },
    "gpt4": {
      "provider": "openai",
      "model": "gpt-4o",
      "max_context_tokens": 128000,
      "info": { "input_cost_per_1m": 2.50, "output_cost_per_1m": 10.00 }
    },
    "local": {
      "provider": "ollama",
      "host": "http://localhost:11434",
      "model": "llama3.2"
    }
  }
}
```

### Profile fields

| Field | Description |
|---|---|
| `provider` | `anthropic`, `openai`, or `ollama` |
| `model` | model identifier string |
| `host` | Ollama server URL (ollama only) |
| `max_context_tokens` | context window size in tokens |
| `context_token_limit` | soft limit for token-budget strategy |
| `max_user_messages` | tail strategy: number of past turns to keep |
| `max_output_tokens` | maximum tokens in the response |
| `strategy` | `tail`, `token-budget`, or `summarize` |
| `summarizer_profile` | profile to use for summarization calls |
| `info.input_cost_per_1m` | cost per 1M input tokens (for display) |
| `info.output_cost_per_1m` | cost per 1M output tokens (for display) |

### API keys

| Provider | Primary env var | Override |
|---|---|---|
| Anthropic | `ANTHROPIC_API_KEY` | `LORE_ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` | `LORE_OPENAI_API_KEY` |
| Ollama | — | `LORE_OLLAMA_HOST` (or `host` in profile) |

---

## TUI interface

```
┌──────────────────────────────────────────────────────────────────┐
│ lore │ topic: dev · model: haiku │ summarize · 84%               │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ● Explain the Builder pattern.              [14:32]  · e to     │
│    The Builder pattern separates object...            expand     │
│                                                                  │
│  ● Give me a Go example.                                         │
│    ❄ streaming ●●●●●                                            │
│    type Server struct { ... }                                    │
│                                                                  │
├──────────────────────────────────────────────────────────────────┤
│ dev/haiku>                                                        │
├──────────────────────────────────────────────────────────────────┤
│ [ #2 ]   dev: 18 calls · $0.02   total: 187 calls · $0.54        │
└──────────────────────────────────────────────────────────────────┘
```

### Zones (top to bottom)

**Top bar** — always visible. Shows program name, active topic, active model, context strategy, and context fill percentage. The fill percentage turns bold yellow when below 10% remaining capacity.

**Conversation pane** — scrollable. Each exchange is a user turn followed immediately by the assistant reply. One blank line separates exchanges. Tab into this pane to navigate exchanges with the arrow keys.

**Input pane** — the prompt field. Shows `<topic>/<model>>` as a prefix. Grows vertically as you type. Type `/` for command completion.

**Status / command pane** — single line by default, showing the `[ #N ]` navigation indicator (when conversation pane is focused) and cumulative stats. Expands when a slash command produces output or a confirmation is required.

### Indicators

- `❄` — pulsating (bold/dim) while waiting for the first streaming token
- `❄ streaming ●●●●●` — brightness wave sweeping across the string while tokens arrive
- `❄ speaking #N ●●●●●` — same wave while TTS is playing an exchange
- `♪` — appended to the box header timestamp of the exchange currently being spoken
- `[ #N ]` — shows which exchange is focused in nav mode

### Themes

Three built-in themes, selected automatically or via flag/command:

| Theme | Description |
|---|---|
| `dark` | Nord palette — cool blues (default for dark backgrounds) |
| `light` | Optimised for light-background iTerm2 profiles |
| `auto` | Detects from `COLORFGBG` at startup (default) |

**At launch:**
```bash
lore --theme light
lore --theme dark
lore --theme auto   # default
```

**Mid-session:**
```
/theme light
/theme dark
/theme auto
/theme options     # list available themes
/theme             # show current mode
```

---

## Key bindings

### Input pane

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |
| `↑` / `↓` | Browse input history (↑ from empty field starts browsing) |
| `Esc` | Clear input field; dismiss completion list; collapse command pane |
| `Tab` | Fill selected completion into input (without executing) |
| `Ctrl+N` | Toggle focus between input and conversation pane |
| `Ctrl+C` | First press: cancel streaming. Within 500 ms again: quit |
| `Ctrl+L` | Clear screen |

### Conversation pane (enter with Ctrl+N, exit with Ctrl+N / Esc / Enter)

| Key | Action |
|---|---|
| `↑` / `↓` | Move focus between exchanges; scrolls viewport at boundaries |
| `v` | Expand / collapse the focused entry (long entries only) |
| `d` | Delete focused exchange — shows confirmation in command pane |
| `s` | Speak focused exchange via TTS; press again to stop |
| `Ctrl+N` / `Esc` / `Enter` | Return focus to input pane |

### Mouse

| Action | Effect |
|---|---|
| Scroll wheel | Scrolls conversation pane |

### Command pane (when expanded)

| Key | Action |
|---|---|
| Any key | Dismiss command pane; return to input |
| `Esc` / `Enter` | Dismiss command pane; return to input |
| Type `yes` + `Enter` | Confirm a pending destructive action |
| Any other input + `Enter` | Cancel a pending action |

---

## Slash commands

Type `/` in the input pane to see completions. Commands can also be typed bare (without `/`) if they are one or two words and the first word matches a known command name.

### Topic

| Command | Description |
|---|---|
| `/topic [name]` | Show info for current topic (or named topic) |
| `/topic-switch <name>` | Switch to an existing topic |
| `/topic-new <name>` | Create a new topic and switch to it |
| `/topic-list` | List all topics |
| `/topic-delete [name]` | Delete a topic (confirmation required) |
| `/topic-clear` | Erase history for current topic (confirmation required) |
| `/topic-default` | Show the configured default topic |
| `/topic-default-set <name>` | Persist a new default topic to config |
| `/topic-summary` | Show the current context summary (if any) |
| `/topic-history [n]` | Show the last N exchanges in plain text (default 10) |

### Resource

| Command | Description |
|---|---|
| `/resource-list [topic]` | List attached files — name, size, modification time |
| `/resource-add <file>` | Copy a file into the current topic's `resources/` directory |
| `/resource-remove <name>` | Remove a resource by filename (confirmation required) |

### Profile

| Command | Description |
|---|---|
| `/profile [code]` | Show info for current profile (or named profile) |
| `/profile-switch <code>` | Switch to a named profile |
| `/profile-list` | List all configured profiles in a table |
| `/profile-default` | Show the configured default profile |
| `/profile-default-set <code>` | Persist a new default profile to config |

### System prompt

| Command | Description |
|---|---|
| `/system` | Show the current system prompt |
| `/system-set <text>` | Set the system prompt for the current topic |
| `/system-clear` | Remove the system prompt |

### Info / utility

| Command | Description |
|---|---|
| `/config` | Show resolved configuration (profiles, roots, defaults) |
| `/status` | Show effective topic, profile, and lore home |
| `/stats` | Show cumulative usage and cost stats |
| `/delete-last [n]` | Delete the last N exchanges from history (default 1) |
| `/fold-all` | Expand or collapse all long entries (toggle) |
| `/play-all` | Play all exchanges via TTS in sequence (toggle — stops if running) |
| `/block-keys` | Show keys available when a block is focused (nav mode) |
| `/help [group]` | Show all commands or commands for a group |
| `/exit` | Exit lore |

### Notes

```
// This is a personal note
```

Any input starting with `//` is saved as a personal note to the current topic's history. Notes are visible in the conversation pane (shown in user-text colour with a 📌 prefix) but are **never sent to the LLM** — they do not consume context tokens and do not influence replies.

---

## File injection (@ref)

Append one or more `@ref` tokens to any prompt to inject file content. The surrounding text becomes the instruction; each referenced file is appended as a named block. Multiple refs are resolved left-to-right.

```
explain this @main.go
compare @old.py and @new.py and summarize the differences
review the spec @~/docs/spec.md with reference to @./impl.go
```

### Resolution rules

| Ref form | Resolves to |
|---|---|
| `@name` | `<topics_root>/<topic>/resources/name` |
| `@subdir/name` | `<topics_root>/<topic>/resources/subdir/name` |
| `@./path` or `@../path` | Relative filesystem path (from current directory) |
| `@/absolute/path` | Absolute filesystem path |
| `@~/path` | Home-relative filesystem path |

Bare names (no leading `/`, `./`, `../`, `~/`) always look up the active topic's `resources/` directory first — this is the primary workflow: add a file once with `/resource-add`, then reference it by name in any future prompt.

### Assembled format

The message sent to the engine is built as:

```
<instruction with @refs stripped>

[file: name1]
<content of file1>

[file: name2]
<content of file2>
```

The filename in the header is always the basename — no filesystem paths leak into the LLM context.

### Error behaviour

If **any** ref cannot be resolved, the entire send is aborted. An error is shown in the command pane and the input text is preserved for correction. There is no partial-send behaviour.

### Display

Assembled messages that exceed 512 characters are auto-folded in the conversation pane. Press `v` (in nav mode) to expand/collapse, or use `/fold-all` to toggle all long entries.

### Works in headless mode

```bash
lore 'explain this' @main.go
lore 'compare @old.py and @new.py'
echo 'what is wrong here?' | lore  # @refs in piped input also work
```

---

## Resources

Each topic has a `resources/` directory under its data folder. Files stored here can be referenced by bare name in `@ref` syntax without typing a full path.

```bash
# Add a file to the current topic
lore --resource-add ./architecture.md
lore -u ./architecture.md              # short form

# List resources
lore --resource-list
lore --resource-list --topic other-topic

# Remove a resource (prompts for confirmation)
lore --resource-remove architecture.md
lore --resource-remove architecture.md --force   # skip prompt
```

In the TUI:

```
/resource-add ~/docs/api-spec.md
/resource-list
/resource-remove api-spec.md
```

After adding a file, reference it in any prompt:

```
summarize the key points from @api-spec.md
```

---

## Headless mode

`lore` runs without the TUI when any of the following is true:

- **stdin is a pipe** — detected automatically
- **`--no-tui` / `-nw`** — explicit flag
- **any admin flag** is present

In headless mode the response streams to stdout; warnings and stats go to stderr.

### One-shot prompts

```bash
lore "what is 2+2"
lore -t myproject "summarize the recent changes"
lore -p sonnet "write a haiku about Go"
echo "explain this code" | lore
lore --input-file prompt.txt
lore --no-stream "prompt"            # collect full response before printing
lore --json "prompt"                 # output as JSON
lore --quiet "prompt"                # suppress stats on stderr
lore --skip-history "prompt"         # one-shot, don't persist to history
lore --all-profiles "prompt"         # run against every configured profile
```

### Topic management

```bash
lore --topic-list
lore --topic-new myproject
lore --topic-info
lore --topic-info -t myproject
lore --topic-history
lore --topic-history --size 5
lore --topic-summary
lore --topic-clear --force
lore --topic-delete --force
lore --topic-default-set myproject
```

### Profile management

```bash
lore --profile-list
lore --profile-default-set sonnet
```

### System prompts

```bash
lore --system                              # show current system prompt
lore --system-set "You are a Go expert."
lore --system-file ./prompts/go-expert.txt
```

### Resource management

```bash
lore --resource-list
lore --resource-list -t myproject
lore --resource-add ./spec.md
lore -u ./spec.md                          # short form
lore --resource-remove spec.md
lore --resource-remove spec.md --force
```

### Notes and history edits

```bash
lore --note "decided to use PostgreSQL"
lore --delete-last                         # delete last exchange
lore --delete-last 3                       # delete last 3 exchanges
lore --delete-last --force                 # skip confirmation
```

### Info and diagnostics

```bash
lore --config
lore --status
lore --stats
lore --debug "prompt"                      # print full request/response to stderr
lore --help-for all
lore --help-for files
lore --help-for topic
```

### Scripting examples

```bash
# Summarise a file and save to disk
lore "summarise this document" @./report.md > summary.txt

# Ask about multiple files
lore "what do these two configs have in common?" @prod.yaml @staging.yaml

# Pipe a command's output into lore
git diff HEAD~1 | lore "write a one-line commit message for this diff"

# Batch a prompt across all profiles (useful for benchmarking)
lore --all-profiles "write a haiku about distributed systems"
```

### Session overrides

```bash
lore --strategy tail --history-window 5 "prompt"
lore --strategy token-budget --context-limit 50000 "prompt"
lore --strategy summarize -p sonnet "prompt"
```

---

## Context strategies

Controls how much conversation history is included in each request.

### tail

Keeps the last N user turns (default: `window_messages` from config, typically 1024). Fast and predictable. Best for short sessions or when full history fits in the context window.

```json
{ "strategy": "tail", "max_user_messages": 20 }
```

### token-budget

Keeps the most recent messages that fit within a token limit. Messages are dropped from the oldest end first.

```json
{ "strategy": "token-budget", "max_context_tokens": 200000, "context_token_limit": 150000 }
```

### summarize

Compresses older history into a rolling summary via a secondary LLM call. New turns are appended verbatim; when the context fills, the oldest un-summarised turns are summarised and the summary is prepended to the context window.

```json
{
  "strategy": "summarize",
  "max_context_tokens": 200000,
  "summarizer_profile": "haiku"
}
```

The `summarizer_profile` is optional; if omitted, the same profile is used for both chat and summarisation.

### Priority order

Per-session flag > profile config > auto-detection (falls back to `tail`).

```bash
lore --strategy summarize --context-limit 80000 "prompt"
```

---

## Text-to-speech

Optional. Requires `~/dev/bin/tts-play` on PATH.

### TUI

**Manual playback:**

| Action | How |
|---|---|
| Speak focused exchange | `s` (in conversation pane nav mode) |
| Stop playback | `s` again, or `Ctrl+C` |
| Play all exchanges in sequence | `/play-all` |
| Stop play-all | `/play-all` again, or `s`, or `Ctrl+C` |

**Auto-mode** — speak every response automatically as it finishes streaming:

| Command | Effect |
|---|---|
| `/tts on` | Enable auto-mode |
| `/tts off` | Disable auto-mode; stops any in-flight playback |
| `/tts` | Toggle auto-mode |

When auto-mode is on and nothing is playing, the status bar shows a `♪ auto` badge. While speaking, it shows `❄ speaking #N ●●●●●` with a brightness wave animation. The active exchange's box header also shows `♪`.

Content passed to `tts-play`:
- Regular exchange: user message + blank line + assistant reply
- Note: note text only
- Raw content is passed as-is via stdin; `tts-play` handles any text cleansing

### Headless

TTS is not invoked automatically in headless mode.

---

## Providers

### Anthropic

```json
{
  "provider": "anthropic",
  "model": "claude-haiku-4-5-20251001",
  "max_context_tokens": 200000
}
```

API key: `ANTHROPIC_API_KEY` (or `LORE_ANTHROPIC_API_KEY` to override).

### OpenAI

```json
{
  "provider": "openai",
  "model": "gpt-4o",
  "max_context_tokens": 128000
}
```

API key: `OPENAI_API_KEY` (or `LORE_OPENAI_API_KEY` to override).

### Ollama

```json
{
  "provider": "ollama",
  "host": "http://localhost:11434",
  "model": "llama3.2"
}
```

Host override: `LORE_OLLAMA_HOST`. No API key required.

---

## Data layout

```
~/.lore/
├── config.json
├── usage.log                        ← append-only cost/token log
└── topics/
    └── <topic-name>/
        ├── history.json             ← full message history
        ├── system.txt               ← system prompt (optional)
        ├── summary.txt              ← rolling summary (summarize strategy)
        └── resources/               ← attached files
            ├── spec.md
            └── data.csv
```

`LORE_HOME` overrides the root directory:

```bash
LORE_HOME=~/work/.lore lore
```

`history.json` uses the same format as `ask` — topic data can be copied between `~/.ask/topics/` and `~/.lore/topics/` without conversion.

---

## Build reference

```bash
make build      # build → ./bin/lore
make install    # test + build + copy to ~/dev/bin/lore
make test       # run unit tests
make testv      # verbose tests
make fmt        # gofmt all files
make vet        # go vet
make check      # fmt-check + vet + lint + test
make clean      # remove build artifacts
```
