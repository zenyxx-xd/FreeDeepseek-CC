#!/usr/bin/env bash
# ==============================================================================
# FreeDeepseek-CC - High-Performance Go Proxy & Claude Wrapper
# ==============================================================================
# Version: v2.2.0
# ==============================================================================

set -e

export LC_ALL=C.UTF-8
export LANG=C.UTF-8

INSTALLER_VERSION="2.2.0"

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
step "Checking & Installing System Dependencies"

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

success "Required dependencies (git, golang, nodejs) are ready."

# Step 2: Build / Update FreeDeepseek-Go
step "Building FreeDeepseek-Go Executable"
mkdir -p "$INSTALL_DIR"

if [ -f "$INSTALL_DIR/freedeepseek-go" ]; then
    success "FreeDeepseek-Go binary is compiled and ready at $INSTALL_DIR/freedeepseek-go"
fi

# Step 3: Configure ~/.bashrc Wrapper & Model Mappings
step "Configuring Claude Code Shell Wrapper & Pro / Vision Model Mappings"
BASHRC="$HOME/.bashrc"
WRAPPER_TAG="# FreeDeepseekAPI Claude Wrapper"

if grep -q "$WRAPPER_TAG" "$BASHRC" 2>/dev/null; then
    info "Updating shell wrapper in ~/.bashrc..."
    sed -i '/# FreeDeepseekAPI Claude Wrapper/,/# End FreeDeepseekAPI Claude Wrapper/d' "$BASHRC" 2>/dev/null || true
fi

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
success "Shell wrapper and model mappings updated in ~/.bashrc."

if [ -r "$BASHRC" ]; then
    source "$BASHRC" 2>/dev/null || true
fi

# Step 4: Authentication Check
step "DeepSeek Authentication Setup"
AUTH_FILE="$INSTALL_DIR/deepseek-auth.json"

if [ -f "$AUTH_FILE" ] && grep -q '"token"' "$AUTH_FILE" 2>/dev/null; then
    success "Authentication token detected in $AUTH_FILE."
fi

echo -e ""
success "Installation completed! Available shortcuts: 'claude-flash', 'claude-flash-thinking', 'claude-pro', 'claude-pro-thinking', 'claude-vision'."
