#!/usr/bin/env bash
# ==============================================================================
# FreeDeepseek-CC - High-Performance Go Proxy & Claude Wrapper
# ==============================================================================
# Version: v2.2.1
# ==============================================================================

set -e

export LC_ALL=C.UTF-8
export LANG=C.UTF-8

INSTALLER_VERSION="2.2.1"

# ANSI Colors
CYAN='\033[38;5;39m'
CYAN_BOLD='\033[1;38;5;39m'
PURPLE_BOLD='\033[1;38;5;141m'
GREEN_BOLD='\033[1;38;5;48m'
RED_BOLD='\033[1;38;5;196m'
GRAY='\033[38;5;242m'
WHITE='\033[1;37m'
RESET='\033[0m'

DIM='\033[2m'

get_cols() {
    local c=""
    if [ -r /dev/tty ]; then c=$(stty size </dev/tty 2>/dev/null | awk '{print $2}' || true); fi
    if [ -z "$c" ]; then c=$(tput cols </dev/tty 2>/dev/null || true); fi
    if [ -z "$c" ] || ! [[ "$c" =~ ^[0-9]+$ ]] || [ "$c" -lt 25 ]; then c="${TERM_WIDTH:-${COLUMNS:-50}}"; fi
    if ! [[ "$c" =~ ^[0-9]+$ ]] || [ "$c" -lt 25 ]; then c=50; fi
    echo "$c"
}

wrap_log() {
    local prefix_vis_len="$1"; local prefix_str="$2"; local indent_str="$3"; local indent_vis_len="$4"; local text="$5"
    local cols=$(get_cols)
    local max_first=$(( cols - prefix_vis_len - 1 )); local max_cont=$(( cols - indent_vis_len - 1 ))
    if [ $max_first -lt 15 ]; then max_first=15; fi; if [ $max_cont -lt 15 ]; then max_cont=15; fi
    local words=($text); local line=""; local is_first=1
    for word in "${words[@]}"; do
        if [ ${#line} -eq 0 ]; then line="$word"; else
            local cur_limit=$max_first; if [ $is_first -eq 0 ]; then cur_limit=$max_cont; fi
            if [ $(( ${#line} + 1 + ${#word} )) -le $cur_limit ]; then line="$line $word"; else
                if [ $is_first -eq 1 ]; then echo -e "${prefix_str}${line}${RESET}"; is_first=0; else echo -e "${indent_str}${line}${RESET}"; fi
                line="$word"
            fi
        fi
    done
    if [ ${#line} -gt 0 ]; then
        if [ $is_first -eq 1 ]; then echo -e "${prefix_str}${line}${RESET}"; else echo -e "${indent_str}${line}${RESET}"; fi
    fi
}

wrap_inst() {
    local prefix_vis_len="$1"; local prefix_str="$2"; local indent_vis_len="$3"; local text_color="$4"; local text="$5"
    local cols=$(get_cols)
    local max_first=$(( cols - prefix_vis_len - 1 )); local max_cont=$(( cols - indent_vis_len - 1 ))
    if [ $max_first -lt 15 ]; then max_first=15; fi; if [ $max_cont -lt 15 ]; then max_cont=15; fi
    local words=($text); local line=""; local is_first=1
    
    local indent_str=""
    for ((i=0; i<indent_vis_len; i++)); do indent_str="${indent_str} "; done

    for word in "${words[@]}"; do
        if [ ${#line} -eq 0 ]; then line="$word"; else
            local cur_limit=$max_first; if [ $is_first -eq 0 ]; then cur_limit=$max_cont; fi
            if [ $(( ${#line} + 1 + ${#word} )) -le $cur_limit ]; then line="$line $word"; else
                if [ $is_first -eq 1 ]; then echo -e "${prefix_str}${text_color}${line}${RESET}"; is_first=0; else echo -e "${indent_str}${text_color}${line}${RESET}"; fi
                line="$word"
            fi
        fi
    done
    if [ ${#line} -gt 0 ]; then
        if [ $is_first -eq 1 ]; then echo -e "${prefix_str}${text_color}${line}${RESET}"; else echo -e "${indent_str}${text_color}${line}${RESET}"; fi
    fi
}

step()    { echo -e ""; wrap_log 3 "◆  ${PURPLE_BOLD}" "   ${PURPLE_BOLD}└─ ${RESET}${PURPLE_BOLD}" 6 "$1"; }
info()    { wrap_log 6 "   ${CYAN}ℹ${RESET}  ${DIM}" "      ${DIM}└─ ${RESET}${DIM}" 9 "$1"; }
success() { wrap_log 6 "   ${GREEN_BOLD}✓${RESET}  ${WHITE}" "      ${DIM}└─ ${RESET}${WHITE}" 9 "$1"; }
error()   { wrap_log 6 "   ${RED_BOLD}✗  Error: ${RESET}${RED_BOLD}" "      ${DIM}└─ ${RESET}${RED_BOLD}" 9 "$1"; }

draw_banner() {
    local ver="$1"
    local term_w=$(get_cols)
    local max_w=$((term_w - 4))
    if [ "$max_w" -lt 24 ]; then max_w=24; fi

    local hline=""
    for ((i=0; i<max_w; i++)); do hline="${hline}─"; done

    pad_text() {
        local text="$1"; local vis_len="$2"
        local pad_len=$(( max_w - vis_len - 2 ))
        if [ "$pad_len" -lt 0 ]; then pad_len=0; fi
        local pad_str=""
        for ((i=0; i<pad_len; i++)); do pad_str="${pad_str} "; done
        echo -n "${text}${pad_str}"
    }

    echo -e "\n${CYAN_BOLD}  ┌${hline}┐${RESET}"
    echo -e "${CYAN_BOLD}  │ $(pad_text "${GRAY}FREE DEEPSEEK GO PROXY${RESET}" 23) ${CYAN_BOLD}│${RESET}"
    echo -e "${CYAN_BOLD}  ├${hline}┤${RESET}"
    echo -e "${CYAN_BOLD}  │ $(pad_text "${GRAY}Version        : ${RESET}${GREEN_BOLD}v${ver}" $(( 18 + ${#ver} ))) ${CYAN_BOLD}│${RESET}"
    echo -e "${CYAN_BOLD}  └${hline}┘${RESET}"
}

draw_instruction_text() {
    step "TOKEN INSTRUCTION / ИНСТРУКЦИЯ ПО ТОКЕНУ"
    wrap_inst 6 "   ${CYAN}ℹ${RESET}  " 6 "\033[1;36m" "1. Log in to chat.deepseek.com in browser"
    wrap_inst 6 "      " 6 "\033[38;5;242m" "Авторизуйтесь на сайте chat.deepseek.com"
    echo -e ""
    wrap_inst 6 "   ${CYAN}ℹ${RESET}  " 6 "\033[1;36m" "2. Copy JS snippet below into address bar:"
    wrap_inst 6 "      " 6 "\033[38;5;242m" "Скопируйте JS-код ниже в адресную строку:"
    echo -e ""
    wrap_inst 6 "   ${RED_BOLD}⚠️  " 6 "\033[1;31m" "IMPORTANT / ВАЖНО:"
    wrap_inst 6 "      " 6 "\033[38;5;208m" "Chrome strips 'javascript:' at start when pasting! Type 'javascript:' manually before pasting snippet."
    wrap_inst 6 "      " 6 "\033[38;5;242m" "Chrome при вставке удаляет 'javascript:' в начале! Напечатайте 'javascript:' вручную в начале строки адреса."
    echo -e ""
    wrap_inst 6 "   ${CYAN}ℹ${RESET}  " 6 "\033[1;36m" "3. Copy token/JSON from popup & paste below"
    wrap_inst 6 "      " 6 "\033[38;5;242m" "Скопируйте полученный токен/JSON из всплывающего окна и вставьте его"
    echo -e ""
    wrap_inst 6 "   ${CYAN}ℹ${RESET}  " 6 "\033[1;36m" "JS SNIPPET FOR COPYING / СКОПИРУЙТЕ СТРОКУ НИЖЕ:"
    echo -e "\033[1;33mjavascript:(function(){var r=localStorage.getItem('userToken')||'',t=r;try{var p=JSON.parse(r);if(p&&p.value)t=p.value}catch(e){}var o={token:t,hif_dliq:localStorage.getItem('hif_dliq')||'',hif_leim:localStorage.getItem('hif_leim')||'',cookie:document.cookie||'',wasmUrl:'https://fe-static.deepseek.com/chat/static/sha3_wasm_bg.7b9ca65ddd.wasm'};prompt('DeepSeek Auth JSON:',JSON.stringify(o,null,2))})()\033[0m\n"
}

on_host_interrupt() {
    trap - SIGINT SIGTERM
    echo -e "\n${RED_BOLD}✗  Installation aborted by user.${RESET}"
    exit 130
}
trap on_host_interrupt SIGINT SIGTERM

clear || true
draw_banner "$INSTALLER_VERSION"

INSTALL_DIR="$HOME/freedeepseek-go"

# Step 1: Check & Auto-install dependencies
step "Step 1/5: Checking & Installing System Dependencies"

hash -r 2>/dev/null || true

MISSING_PKGS=()
if ! command -v git >/dev/null 2>&1; then
    MISSING_PKGS+=("git")
fi
if ! command -v go >/dev/null 2>&1; then
    MISSING_PKGS+=("golang")
fi
if ! command -v node >/dev/null 2>&1; then
    MISSING_PKGS+=("nodejs")
fi

if [ ${#MISSING_PKGS[@]} -ne 0 ]; then
    info "Missing required packages (${MISSING_PKGS[*]}). Installing automatically..."
    export DEBIAN_FRONTEND=noninteractive
    if command -v pkg >/dev/null 2>&1; then
        pkg install -y "${MISSING_PKGS[@]}" >/dev/null 2>&1 || apt-get install -y "${MISSING_PKGS[@]}" >/dev/null 2>&1 || true
    elif command -v apt-get >/dev/null 2>&1; then
        apt-get update -y >/dev/null 2>&1 || true
        apt-get install -y "${MISSING_PKGS[@]}" >/dev/null 2>&1 || true
    fi
    hash -r 2>/dev/null || true
fi

# Re-verify packages
STILL_MISSING=()
if ! command -v git >/dev/null 2>&1; then STILL_MISSING+=("git"); fi
if ! command -v go >/dev/null 2>&1; then STILL_MISSING+=("golang"); fi
if ! command -v node >/dev/null 2>&1; then STILL_MISSING+=("nodejs"); fi

if [ ${#STILL_MISSING[@]} -ne 0 ]; then
    error "Failed to install missing packages: ${STILL_MISSING[*]}."
    exit 1
fi

success "Required dependencies (git, golang, nodejs) are installed and ready."

# Step 2: Clone / Update Repository
step "Step 2/5: Synchronizing FreeDeepseek-CC Repository"
info "Target directory: $INSTALL_DIR"

if [ -d "$INSTALL_DIR/.git" ]; then
    info "Existing repository detected. Pulling latest code..."
    (cd "$INSTALL_DIR" && git pull >/dev/null 2>&1 || true)
    success "Repository updated successfully."
else
    info "Cloning https://github.com/zenyxx-xd/FreeDeepseek-CC.git into $INSTALL_DIR..."
    mkdir -p "$(dirname "$INSTALL_DIR")"
    git clone https://github.com/zenyxx-xd/FreeDeepseek-CC.git "$INSTALL_DIR" >/dev/null 2>&1 || mkdir -p "$INSTALL_DIR"
    success "Repository synchronized."
fi

# Step 3: Compile Go Binary
step "Step 3/5: Compiling High-Performance Go Proxy Binary"
if [ -d "$INSTALL_DIR" ]; then
    info "Compiling freedeepseek-go executable..."
    if [ -f "$INSTALL_DIR/main.go" ]; then
        (cd "$INSTALL_DIR" && go build -o freedeepseek-go main.go >/dev/null 2>&1 || true)
    fi
fi

if [ -f "$INSTALL_DIR/freedeepseek-go" ]; then
    success "Compiled binary ready at $INSTALL_DIR/freedeepseek-go."
else
    error "Go binary compilation failed. Please verify Go environment."
fi

# Step 4: Configure Shell Wrapper & Aliases
step "Step 4/5: Configuring Shell Wrapper & Model Aliases"
BASHRC="$HOME/.bashrc"
WRAPPER_TAG="# FreeDeepseekAPI Claude Wrapper"

if grep -q "$WRAPPER_TAG" "$BASHRC" 2>/dev/null; then
    info "Updating existing shell wrapper block in ~/.bashrc..."
    sed -i '/# FreeDeepseekAPI Claude Wrapper/,/# End FreeDeepseekAPI Claude Wrapper/d' "$BASHRC" 2>/dev/null || true
fi

info "Appending wrapper function and model aliases to ~/.bashrc..."
cat << 'EOF_BASHRC' >> "$BASHRC"

# FreeDeepseekAPI Claude Wrapper
claude() {
    local PROXY_BIN="$HOME/freedeepseek-go/freedeepseek-go"
    if ! pgrep -f "freedeepseek-go" >/dev/null 2>&1; then
        echo -e "\033[1;36m[FreeDeepseek-Go]\033[0m Starting high-performance Go proxy server..."
        (cd "$(dirname "$PROXY_BIN")" && "$PROXY_BIN" >/dev/null 2>&1 &)
        sleep 1
    fi
    export ANTHROPIC_BASE_URL="http://localhost:9655"
    export ANTHROPIC_DEFAULT_HAIKU_MODEL="DeepSeek Flash"
    export ANTHROPIC_DEFAULT_SONNET_MODEL="DeepSeek Pro Thinking"
    export ANTHROPIC_DEFAULT_OPUS_MODEL="DeepSeek Pro"
    export ANTHROPIC_DEFAULT_FABLE_MODEL="DeepSeek Pro Thinking"
    command claude "$@"
}

# DeepSeek Model Aliases for Claude Code
alias claude-flash='claude --model "DeepSeek Flash"'
alias claude-flash-thinking='claude --model "DeepSeek Flash Thinking"'
alias claude-pro='claude --model "DeepSeek Pro"'
alias claude-pro-thinking='claude --model "DeepSeek Pro Thinking"'
alias claude-vision='claude --model "DeepSeek Vision"'
# End FreeDeepseekAPI Claude Wrapper
EOF_BASHRC
success "Shell wrapper and model aliases successfully configured in ~/.bashrc."

if [ -r "$BASHRC" ]; then
    source "$BASHRC" 2>/dev/null || true
fi

# Step 5: DeepSeek Token Authentication Setup
step "Step 5/5: Mobile DeepSeek Token Setup"

AUTH_FILE="$INSTALL_DIR/deepseek-auth.json"

if [ -f "$AUTH_FILE" ] && grep -q '"token"' "$AUTH_FILE" 2>/dev/null; then
    success "Existing authentication token detected in $AUTH_FILE."
    wrap_inst 6 "   ℹ  " 6 "\033[38;5;242m" "To update token later, edit $AUTH_FILE or delete it and re-run installer."
    wrap_inst 6 "      " 6 "\033[38;5;242m" "(Чтобы обновить токен позже, отредактируйте $AUTH_FILE или удалите его и запустите установщик снова)."
else
    draw_instruction_text

    wrap_inst 6 "   👉 " 6 "\033[1;38;5;48m" "Read instruction above and press Enter to open chat.deepseek.com in browser..."
    wrap_inst 6 "      " 6 "\033[38;5;242m" "Прочитайте инструкцию выше и нажмите Enter, чтобы открыть chat.deepseek.com в браузере..."
    read -r _unused_input

    echo -e ""
    wrap_inst 6 "   ${CYAN}ℹ${RESET}  " 6 "\033[1;36m" "Opening https://chat.deepseek.com in your mobile browser..."

    if command -v termux-open >/dev/null 2>&1; then
        termux-open "https://chat.deepseek.com" >/dev/null 2>&1 || true
    fi

    echo -e ""
    wrap_inst 6 "   🔑 " 6 "\033[1;36m" "PASTE YOUR DEEPSEEK TOKEN / ВСТАВЬТЕ ВАШ DEEPSEEK TOKEN:"
    read -p "Token/JSON: " USER_TOKEN

    if [ -n "$USER_TOKEN" ]; then
        if command -v node >/dev/null 2>&1; then
            node -e '
            var input = process.argv[1] || "";
            var authFile = process.argv[2];

            function extractField(key, str) {
              var re = new RegExp("\"" + key + "\"\\s*:\\s*\"([^\"]*)\"");
              var m = str.match(re);
              return m ? m[1] : "";
            }

            function extractCleanToken(str) {
              var matchVal = str.match(/"value"\s*:\s*"([^"]+)"/);
              if (matchVal && matchVal[1]) return matchVal[1];

              var matchTok = str.match(/"token"\s*:\s*"([^"]+)"/);
              if (matchTok && matchTok[1] && !matchTok[1].includes("{")) return matchTok[1];

              var cur = str;
              for (var i = 0; i < 5; i++) {
                try {
                  var p = JSON.parse(cur);
                  if (typeof p === "string") cur = p;
                  else if (p && typeof p === "object") {
                    if (p.value) cur = p.value;
                    else if (p.token) cur = p.token;
                    else break;
                  } else break;
                } catch(e) {
                  break;
                }
              }
              return String(cur).replace(/^["\x27\s{}]+|["\x27\s{}]+$/g, "").trim();
            }

            var token = extractCleanToken(input);
            var cookie = extractField("cookie", input);
            var hif_dliq = extractField("hif_dliq", input);
            var hif_leim = extractField("hif_leim", input);
            var wasmUrl = extractField("wasmUrl", input) || "https://fe-static.deepseek.com/chat/static/sha3_wasm_bg.7b9ca65ddd.wasm";

            var authObj = {
              token: token,
              hif_dliq: hif_dliq,
              hif_leim: hif_leim,
              cookie: cookie,
              wasmUrl: wasmUrl
            };

            require("fs").writeFileSync(authFile, JSON.stringify(authObj, null, 2));
            ' "$USER_TOKEN" "$AUTH_FILE"
        else
            USER_TOKEN=$(echo "$USER_TOKEN" | tr -d '"' | tr -d "'" | tr -d ' ')
            cat << EOF_AUTH > "$AUTH_FILE"
{
  "token": "$USER_TOKEN",
  "hif_dliq": "",
  "hif_leim": "",
  "cookie": "",
  "wasmUrl": "https://fe-static.deepseek.com/chat/static/sha3_wasm_bg.7b9ca65ddd.wasm"
}
EOF_AUTH
        fi
        chmod 600 "$AUTH_FILE"
        echo -e ""
        success "Authentication file successfully created at $AUTH_FILE!"
    fi
fi

step "Installation Summary"
info "Location: $INSTALL_DIR"
info "Available Aliases: claude-flash, claude-flash-thinking, claude-pro, claude-pro-thinking, claude-vision"

echo -e ""
success "Installation completed successfully!"
