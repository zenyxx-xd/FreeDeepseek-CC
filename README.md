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

## ✨ Key Features (v2.2.0 Go Engine)

- ⚡ **Blazing Fast Go Engine**: Single 9MB standalone Go binary, sub-millisecond local WASM PoW solver, sub-3ms startup latency, and ultra-low RAM usage (~10MB).
- 🔄 **Stateful Delta Protocol**: Tracks Claude Code session IDs (`x-claude-code-session-id`) and DeepSeek `chat_session_id`. Transmits **only new user turns (deltas)** + `parent_message_id`, eliminating 95% of unnecessary prompt resends!
- 🧠 **Dynamic Effort Prompt Injection**: Automatically reads Claude Code's `output_config.effort` (`low`, `medium`, `high`, `xhigh`, `max`) and injects prompt instructions.
- 📱 **Mobile DeepSeek Token Helper**: Includes JS bookmarklet snippet to extract auth JSON from mobile browsers.

---

## 🎯 Supported Models & Shortcuts

| Command Alias | Model Flag Passed | Mode Description |
| :--- | :--- | :--- |
| `claude` | *Default (`DeepSeek Pro Thinking`)* | Standard launch |
| `claude-flash` | `--model "DeepSeek Flash"` | Fast non-thinking chat mode |
| `claude-flash-thinking` | `--model "DeepSeek Flash Thinking"` | Fast mode with R1 reasoning |
| `claude-pro` | `--model "DeepSeek Pro"` | DeepSeek Web Pro mode |
| `claude-pro-thinking` | `--model "DeepSeek Pro Thinking"` | DeepSeek Web Pro mode with R1 reasoning |
| `claude-vision` | `--model "DeepSeek Vision"` | Image & vision understanding mode |

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
