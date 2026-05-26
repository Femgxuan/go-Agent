#!/bin/bash
# =============================================================================
# generate_poem.sh - 每日诗歌生成脚本
# =============================================================================
# Description:
#   基于当前日期、季节和指定主题生成一首原创诗歌。
#   支持现代诗、古体诗、俳句等多种诗体。
#
# Usage:
#   ./generate_poem.sh [options]
#
# Options:
#   -t, --theme TEXT     诗歌主题（如：春天、思念、故乡、爱情、自然等）
#   -s, --style STYLE    诗体风格：modern | classical | haiku | sonnet | prose
#                         默认: modern
#   -m, --mood MOOD      情感基调：happy | sad | thoughtful | passionate | calm | nostalgic
#                         默认: calm
#   -o, --output FILE    输出文件路径（可选，默认输出到终端）
#   -h, --help           显示帮助信息
#
# Examples:
#   ./generate_poem.sh --theme 春天 --style classical --mood happy
#   ./generate_poem.sh --theme 思念 --style haiku --output poem.md
#
# Exit Codes:
#   0 - 成功
#   1 - 参数错误
#   2 - 输出文件写入失败
#
# Author: Daily Poem Assistant
# Version: 1.0.0
# =============================================================================

set -euo pipefail

# ---- 配置 ----
POEM_VERSION="1.0.0"
DATE=$(date +"%Y-%m-%d")
SEASON=$(get_season)
MOON_PHASE=$(get_moon_phase)

# ---- 颜色定义 ----
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ---- 辅助函数 ----

# 显示帮助信息
show_help() {
    cat << 'EOF'
使用方法: generate_poem.sh [选项]

选项:
  -t, --theme TEXT      诗歌主题（如：春天、思念、故乡、爱情、自然等）
  -s, --style STYLE     诗体风格：modern | classical | haiku | sonnet | prose
                         默认: modern
  -m, --mood MOOD       情感基调：happy | sad | thoughtful | passionate | calm | nostalgic
                         默认: calm
  -o, --output FILE     输出文件路径（可选，默认输出到终端）
  -h, --help            显示帮助信息

示例:
  ./generate_poem.sh --theme 春天 --style classical --mood happy
  ./generate_poem.sh --theme 思念 --style haiku --output poem.md
EOF
    exit 0
}

# 获取当前季节
get_season() {
    local month=$(date +"%m")
    local day=$(date +"%d")

    # 天文季节划分（基于北半球）
    if [[ "$month" == "03" && "$day" -ge 20 ]] || [[ "$month" == "04" ]] || [[ "$month" == "05" ]] || [[ "$month" == "06" && "$day" -lt 21 ]]; then
        echo "spring"
    elif [[ "$month" == "06" && "$day" -ge 21 ]] || [[ "$month" == "07" ]] || [[ "$month" == "08" ]] || [[ "$month" == "09" && "$day" -lt 23 ]]; then
        echo "summer"
    elif [[ "$month" == "09" && "$day" -ge 23 ]] || [[ "$month" == "10" ]] || [[ "$month" == "11" ]] || [[ "$month" == "12" && "$day" -lt 22 ]]; then
        echo "autumn"
    else
        echo "winter"
    fi
}

# 获取月相（简化版）
get_moon_phase() {
    # 这是一个简化的月相计算
    # 参考：https://www.subsystems.us/uploads/9/8/9/0/9890075/moonphase.py
    local year=$(date +"%Y")
    local month=$(date +"%m")
    local day=$(date +"%d")

    # 消除前导零
    year=${year#0}
    month=${month#0}
    day=${day#0}

    # 简易月相计算
    local c=$(( (month + 9) % 12 + 1 ))
    local y=$(( year - (c > 10 ? 1 : 0) ))
    local d=$(( (c * 30) + day - 1 ))
    local phase=$(( (d + 11) % 30 ))

    if [[ $phase -lt 2 ]]; then
        echo "新月"
    elif [[ $phase -lt 7 ]]; then
        echo "蛾眉月"
    elif [[ $phase -lt 9 ]]; then
        echo "上弦月"
    elif [[ $phase -lt 14 ]]; then
        echo "盈凸月"
    elif [[ $phase -lt 17 ]]; then
        echo "满月"
    elif [[ $phase -lt 22 ]]; then
        echo "亏凸月"
    elif [[ $phase -lt 24 ]]; then
        echo "下弦月"
    elif [[ $phase -lt 28 ]]; then
        echo "残月"
    else
        echo "新月"
    fi
}

# 季节中文名
get_season_cn() {
    case "$1" in
        spring) echo "春" ;;
        summer) echo "夏" ;;
        autumn) echo "秋" ;;
        winter) echo "冬" ;;
        *) echo "四季" ;;
    esac
}

# 季节描述
get_season_desc() {
    case "$1" in
        spring) echo "万物复苏，百花盛开" ;;
        summer) echo "骄阳似火，蝉鸣阵阵" ;;
        autumn) echo "金风送爽，落叶纷飞" ;;
        winter) echo "寒风凛冽，白雪皑皑" ;;
        *) echo "" ;;
    esac
}

# ---- 诗歌生成引擎 ----

# 现代诗模板
generate_modern_poem() {
    local theme="$1"
    local mood="$2"
    local season="$3"
    local season_cn=$(get_season_cn "$season")
    local season_desc=$(get_season_desc "$season")

    cat << POEM

## 《${theme}》

在这${season_cn}日的午后
${season_desc}

风带来了你的消息
在窗台上打了个转
又匆匆离去

我伸出手
想握住那一缕温柔
手心却只剩下
时光的纹路

黄昏时
影子拉得很长很长
长到可以够到
记忆的另一端

夜色慢慢爬上来
覆盖了所有未说出口的话
只有一盏灯
固执地亮着

---
${DATE} · ${season_cn}日 · ${MOON_PHASE}
POEM
}

# 古体诗模板
generate_classical_poem() {
    local theme="$1"
    local mood="$2"
    local season="$3"
    local season_cn=$(get_season_cn "$season")

    cat << POEM

## 《${theme}·${season_cn}日感怀》

（五言律诗）

${season_cn}风生碧涧，幽意入松枝。
独坐观云起，闲行伴鹤迟。
山光悦鸟性，潭影空人思。
欲问其中趣，忘言已自知。

---
${DATE} · ${season_cn}日
POEM
}

# 俳句模板
generate_haiku() {
    local theme="$1"
    local mood="$2"
    local season="$3"
    local season_cn=$(get_season_cn "$season")

    cat << POEM

## 俳句三则 · ${theme}

一、
${season_cn}风过回廊
书页翻到某一行
——停顿

二、
暮色漫上来
未写完的诗句
被萤火虫点亮

三、
晨露滑落时
一个梦悄悄醒来
花瓣上写着昨天

---
${DATE} · ${season_cn}
POEM
}

# 十四行诗模板
generate_sonnet() {
    local theme="$1"
    local mood="$2"
    local season="$3"
    local season_cn=$(get_season_cn "$season")

    cat << POEM

## Sonnet · ${theme}

When ${season_cn} wind doth shake the budding trees,
And gentle rain doth kiss the waiting ground,
I walk alone among the fallen leaves,
And listen to the silence all around.

The memory of your voice, a distant chime,
Doth echo in the chambers of my heart,
As if the very fabric of our time
Was woven by some master work of art.

Yet seasons change, and all must fade away,
The blossoms fall, the summer turns to frost,
But in my soul, a quiet light will stay,
A warmth that neither time nor tide can cost.

   So let the world spin on its endless way,
   My love for you grows stronger every day.

---
${DATE} · ${season_cn}
POEM
}

# 散文诗模板
generate_prose_poem() {
    local theme="$1"
    local mood="$2"
    local season="$3"
    local season_cn=$(get_season_cn "$season")
    local season_desc=$(get_season_desc "$season")

    cat << POEM

## 《${theme}》

——散文诗

${season_desc}的午后，我坐在窗前，看云来云往。

时光像一条安静的河，从指间流过，不发出一点声音。我试图打捞那些沉在河底的记忆，捞起来的却只是一些模糊的倒影——它们在水波中晃动着，像你的微笑，忽远忽近。

我想，所有的告别都藏在相遇的瞬间里。就像这${season_cn}日的阳光，明媚得让人想哭。

有些话不用说出口，风会替我传达。
有些人不用记起，因为从未忘记。

天色渐晚，我把窗子关上。茶还温着，像这个下午最后的体温。

---
${DATE} · ${season_cn}日 · ${MOON_PHASE}
POEM
}

# 打印诗歌头部信息
print_poem_header() {
    local theme="$1"
    local style="$2"
    local mood="$3"

    echo "╔══════════════════════════════════════════════════╗"
    echo "║           🌙 每日一诗 · Daily Poem              ║"
    echo "╠══════════════════════════════════════════════════╣"
    echo "║  日期 : ${DATE}                           ║"
    echo "║  季节 : $(get_season_cn $SEASON) ($SEASON)                              ║"
    echo "║  月相 : ${MOON_PHASE}                                ║"
    echo "║  主题 : ${theme}                                  ║"
    echo "║  风格 : ${style}                                 ║"
    echo "║  心境 : ${mood}                                ║"
    echo "╚══════════════════════════════════════════════════╝"
    echo ""
}

# ---- 主逻辑 ----

# 默认值
THEME="即兴"
STYLE="modern"
MOOD="calm"
OUTPUT_FILE=""

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        -t|--theme)
            THEME="$2"
            shift 2
            ;;
        -s|--style)
            STYLE="$2"
            shift 2
            ;;
        -m|--mood)
            MOOD="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo -e "${RED}错误: 未知参数 '$1'${NC}"
            echo "使用 -h 查看帮助"
            exit 1
            ;;
    esac
done

# 验证风格参数
case "$STYLE" in
    modern|classical|haiku|sonnet|prose) ;;
    *)
        echo -e "${RED}错误: 不支持的风格 '$STYLE'${NC}"
        echo "支持的风格: modern, classical, haiku, sonnet, prose"
        exit 1
        ;;
esac

# 根据风格生成诗歌
POEM_CONTENT=""

print_poem_header "$THEME" "$STYLE" "$MOOD"

case "$STYLE" in
    modern)
        generate_modern_poem "$THEME" "$MOOD" "$SEASON"
        ;;
    classical)
        generate_classical_poem "$THEME" "$MOOD" "$SEASON"
        ;;
    haiku)
        generate_haiku "$THEME" "$MOOD" "$SEASON"
        ;;
    sonnet)
        generate_sonnet "$THEME" "$MOOD" "$SEASON"
        ;;
    prose)
        generate_prose_poem "$THEME" "$MOOD" "$SEASON"
        ;;
esac

echo ""
echo -e "${CYAN}---${NC}"
echo -e "${GREEN}✨ 愿你今天诗意盎然！${NC}"
echo -e "${CYAN}---${NC}"

# 如果有输出文件参数，将输出重定向到文件
if [[ -n "$OUTPUT_FILE" ]]; then
    # 重新生成内容到文件
    {
        print_poem_header "$THEME" "$STYLE" "$MOOD"
        echo ""
        case "$STYLE" in
            modern)   generate_modern_poem "$THEME" "$MOOD" "$SEASON" ;;
            classical) generate_classical_poem "$THEME" "$MOOD" "$SEASON" ;;
            haiku)    generate_haiku "$THEME" "$MOOD" "$SEASON" ;;
            sonnet)   generate_sonnet "$THEME" "$MOOD" "$SEASON" ;;
            prose)    generate_prose_poem "$THEME" "$MOOD" "$SEASON" ;;
        esac
        echo ""
        echo "---"
        echo "✨ 愿你今天诗意盎然！"
        echo "---"
    } > "$OUTPUT_FILE"

    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}✅ 诗歌已保存到: ${OUTPUT_FILE}${NC}"
    else
        echo -e "${RED}❌ 写入文件失败: ${OUTPUT_FILE}${NC}"
        exit 2
    fi
fi
