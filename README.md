# FreeDeepseek-CC 🚀

> **High-Performance Go Proxy & Claude Code Model Wrapper for DeepSeek Web AI.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Language: Go](https://img.shields.io/badge/Language-Go-00ADD8.svg)](https://golang.org)
[![Platform: Termux / Linux](https://img.shields.io/badge/Platform-Termux%20%7C%20Linux-blue.svg)](https://termux.dev)

**FreeDeepseek-CC** turns [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) into a zero-cost AI coding agent powered by DeepSeek's models via a high-performance **Go** proxy engine.

---

## ⚡ Quick Install

Run this command in your Termux or Linux terminal:

```bash
source <(curl -fsSL https://raw.githubusercontent.com/zenyxx-xd/FreeDeepseek-CC/main/install.sh)
```

---

## ✨ Key Features

- ⚡ **Blazing Fast Go Engine**: Standalone Go binary, sub-millisecond local WASM PoW solver, sub-3ms startup latency, and ultra-low RAM usage (~10MB).
- 🔄 **Stateful Delta Protocol**: Tracks Claude Code session IDs (`x-claude-code-session-id`) and DeepSeek `chat_session_id`. Transmits only new user turns (deltas) + `parent_message_id`, eliminating 95% of unnecessary prompt resends!
- 📋 **Full System Prompt Extraction**: Supports both string and structured block arrays (`[]interface{}`) from Claude Code, ensuring all tool definitions, file editing instructions, and system reminders are preserved 100%.
- 🧠 **Smart Effort Level Control**: Automatically maps Claude Code's `output_config.effort` (`low`, `medium`, `high`, `max`, `ultracode`) to dynamic thinking directives for Thinking models.
- 📱 **Mobile DeepSeek Token Helper**: Includes interactive setup helper to extract auth token JSON from mobile browsers.

---

## 🎯 Supported Models & Shortcuts

| Command Alias | Model Name | Description |
| :--- | :--- | :--- |
| `claude` | *Default (`DeepSeek Pro Thinking`)* | Standard interactive Claude Code TUI session |
| `claude-pro` | `DeepSeek Pro` | DeepSeek Web Expert mode (no reasoning) |
| `claude-pro-thinking` | `DeepSeek Pro Thinking` | DeepSeek Web Expert mode with R1 reasoning |
| `claude-flash` | `DeepSeek Flash` | DeepSeek Web Fast mode (no reasoning) |
| `claude-flash-thinking` | `DeepSeek Flash Thinking` | DeepSeek Web Fast mode with R1 reasoning |
| `claude-vision` | `DeepSeek Vision` | DeepSeek Web Vision mode (no reasoning) |
| `claude-vision-thinking` | `DeepSeek Vision Thinking` | DeepSeek Web Vision mode with R1 reasoning |

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
