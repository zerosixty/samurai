#!/usr/bin/env bash
# Builds 25 (variant × fixture) prompts for the subagent A/B test.
# Each prompt is self-contained: variant SKILL.md inline + shared naming.md + fixture code.

set -euo pipefail

cd "$(dirname "$0")"

mkdir -p runs/prompts
rm -f runs/prompts/*.txt

NAMING_MD=$(cat variants/naming.md)

INSTRUCTIONS='--- END TEST FILE ---

INSTRUCTIONS:

For every `s.Test("...")` string literal in the file (in source order), output ONE block in this EXACT format:

NAME: <copy the name verbatim, no surrounding quotes>
VERDICT: PASS or FLAG
REASON: <which forbidden pattern matched, in 1-6 words; or "clean" if PASS>
REPLACEMENT: <if FLAG, one or two lowercase 3-7-word alternatives separated by " | "; if PASS, write "n/a">

Separate blocks with a SINGLE blank line. Output nothing else — no preamble, no summary, no headers, no commentary.'

for v in v1 v2 v3 v4 v5; do
  variant_content=$(cat "variants/${v}.md")
  for f in F1_function_leak F2_assertion_phrasing F3_structure_leak F4_http_leak F5_good_names_control; do
    fixture_content=$(cat "fixtures/${f}.go.txt")
    out="runs/prompts/${v}__${f}.txt"
    {
      echo "You are reviewing test names in a Go samurai test file. Apply the naming guidance below; output a structured per-name verdict."
      echo
      echo "--- GUIDANCE (inline SKILL.md section) ---"
      echo "$variant_content"
      echo "--- END GUIDANCE ---"
      echo
      echo "--- LAZY-LOADED naming.md (load only if your guidance refers to it or you need the full protocol) ---"
      echo "$NAMING_MD"
      echo "--- END naming.md ---"
      echo
      echo "--- TEST FILE TO REVIEW ---"
      echo "$fixture_content"
      echo "$INSTRUCTIONS"
    } > "$out"
  done
done

echo "Built $(ls runs/prompts | wc -l | tr -d ' ') prompts in runs/prompts/"
ls runs/prompts/ | head -5
