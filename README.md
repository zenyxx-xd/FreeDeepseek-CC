# FreeDeepseek-CC 🚀

> **Automated Installer & Model Mapping Wrapper for [FreeDeepseekAPI](https://github.com/ForgetMeAI/FreeDeepseekAPI) & Claude Code on Android (Termux) & Linux.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Platform: Termux / Linux](https://img.shields.io/badge/Platform-Termux%20%7C%20Linux-blue.svg)](https://termux.dev)
[![Node.js](https://img.shields.io/badge/Node.js-v18%2B-green.svg)](https://nodejs.org)

**FreeDeepseek-CC** turns [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) into a powerful AI coding agent powered by DeepSeek's models for **free**, running smoothly on Termux (Android) and Linux terminals.

---

## ⚡ Quick Install

Run this single command in your Termux or Linux terminal:

```bash
source <(curl -fsSL https://raw.githubusercontent.com/zenyxx-xd/FreeDeepseek-CC/main/install.sh)
```

> [!TIP]
> Using `source <(...)` automatically loads `claude` and all model aliases into your current shell session immediately without restarting your terminal!

---

## ✨ Features

- 📦 **Automated Setup**: Installs system dependencies (`git`, `nodejs`, `npm`), clones `FreeDeepseekAPI`, and configures the background proxy.
- 📱 **Mobile DeepSeek Token Helper**: Provides a 1-click JavaScript bookmarklet snippet to extract your token & authentication JSON directly from Chrome on mobile devices.
- 🤖 **Smart Model Mapping**: Automatically maps Claude model tiers (`haiku`, `sonnet`, `opus`, `fable`) to corresponding DeepSeek models.
- ⚡ **Instant Model Aliases**: Handy terminal shortcuts (`claude-chat`, `claude-reasoner`, `claude-expert`, `claude-v4-pro`) using `claude --model ...`.
- 🛡️ **Fail-safe Token Sanitizer**: Parses, cleans, and formats nested JSON tokens without quotes corruption.
- 🔄 **Auto Proxy Launch**: `claude` wrapper automatically starts the `FreeDeepseekAPI` proxy server on port `9655` in the background if it isn't running already.

---

## 🎯 Model Mappings

When using Claude Code with `FreeDeepseek-CC`, Claude's model selector automatically routes requests to DeepSeek's models:

| Claude Model Tier | Mapped DeepSeek Model | Description / Mode |
| :--- | :--- | :--- |
| **`haiku`** | `deepseek-chat` | Standard fast chat |
| **`sonnet`** | `deepseek-reasoner` | Reasoning / Thinking mode (`R1`) |
| **`opus`** | `deepseek-expert` | Expert mode |
| **`fable`** | `deepseek-v4-pro` | Expert mode with reasoning |

---

## 🖥️ Terminal Shortcuts

After installation, you can launch Claude Code with specific DeepSeek models using these shortcuts:

| Command Shortcut | Model Flag Passed | Usage |
| :--- | :--- | :--- |
| `claude` | *Default (`deepseek-chat`)* | Standard launch |
| `claude-chat` | `--model deepseek-chat` | Fast chat mode |
| `claude-default` | `--model deepseek-default` | Default compatibility alias |
| `claude-reasoner` | `--model deepseek-reasoner` | DeepSeek R1 reasoning mode |
| `claude-r1` | `--model deepseek-r1` | R1 alias |
| `claude-expert` | `--model deepseek-expert` | Expert mode |
| `claude-v4-pro` | `--model deepseek-v4-pro` | Pro expert + reasoning mode |

---

## 🔑 Mobile Browser Token Snippet

Mobile Chrome automatically strips `javascript:` when pasting into the address bar. 

1. Open [chat.deepseek.com](https://chat.deepseek.com) and log in.
2. In the address bar, type `javascript:` manually at the very beginning.
3. Paste the following snippet and press enter:

```javascript
javascript:(function(){var r=localStorage.getItem('userToken')||'',t=r;try{var p=JSON.parse(r);if(p&&p.value)t=p.value}catch(e){}var o={token:t,hif_dliq:localStorage.getItem('hif_dliq')||'',hif_leim:localStorage.getItem('hif_leim')||'',cookie:document.cookie||'',wasmUrl:'https://fe-static.deepseek.com/chat/static/sha3_wasm_bg.7b9ca65ddd.wasm'};prompt('DeepSeek Auth JSON:',JSON.stringify(o,null,2))})()
```

4. Copy the resulting JSON string from the popup dialog and paste it into Termux when prompted by the installer.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
