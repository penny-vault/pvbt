#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STUDY_DIR="$(dirname "$SCRIPT_DIR")"
REPORT_DIR="$STUDY_DIR/report"

cd "$SCRIPT_DIR"

# Gather all .vue files under study/ excluding study/ui/src/components/ and node_modules
COMPONENTS=()
while IFS= read -r -d '' file; do
  COMPONENTS+=("$file")
done < <(find "$STUDY_DIR" -name '*.vue' \
  -not -path '*/node_modules/*' \
  -not -path "$SCRIPT_DIR/src/components/*" \
  -print0 | sort -z)

# Generate src/main.js
MAIN="$SCRIPT_DIR/src/main.js"
cat > "$MAIN" <<'HEADER'
import { createApp, h } from 'vue'
import './style.css'
HEADER

# Import each component
idx=0
declare -a NAMES=()
for comp in "${COMPONENTS[@]+"${COMPONENTS[@]}"}"; do
  name=$(basename "$comp" .vue)
  NAMES+=("$name")
  rel=$(python3 -c "import os.path; p=os.path.relpath('$comp', '$SCRIPT_DIR/src'); print(p if p.startswith('.') else './'+p)")
  echo "import ${name} from '${rel}'" >> "$MAIN"
  idx=$((idx + 1))
done

# Import shared components from src/components/ if they exist
SHARED=()
if [ -d "$SCRIPT_DIR/src/components" ]; then
  while IFS= read -r -d '' file; do
    SHARED+=("$file")
  done < <(find "$SCRIPT_DIR/src/components" -name '*.vue' -print0 | sort -z)
fi

for comp in "${SHARED[@]}"; do
  name=$(basename "$comp" .vue)
  NAMES+=("$name")
  rel=$(python3 -c "import os.path; p=os.path.relpath('$comp', '$SCRIPT_DIR/src'); print(p if p.startswith('.') else './'+p)")
  echo "import ${name} from '${rel}'" >> "$MAIN"
done

# Write the app mount logic
cat >> "$MAIN" <<'BODY'

const components = {
BODY

for name in "${NAMES[@]}"; do
  echo "  ${name}," >> "$MAIN"
done

cat >> "$MAIN" <<'FOOTER'
}

const app = createApp({
  data() {
    return { reportData: window.__REPORT_DATA__ }
  },
  render() {
    const comp = components[window.__REPORT_COMPONENT__]
    if (!comp) {
      return null
    }
    return h(comp, { data: this.reportData })
  },
})

Object.entries(components).forEach(([name, comp]) => {
  app.component(name, comp)
})

app.mount('#app')
FOOTER

echo "Generated $MAIN with ${#NAMES[@]} components"

# Build
npm run build

# Copy output to report directory
cp "$SCRIPT_DIR/dist/bundle.js" "$REPORT_DIR/bundle.js"

# Inline CSS into bundle as a self-injecting style tag
if [ -f "$SCRIPT_DIR/dist/style.css" ]; then
  CSS_CONTENT=$(cat "$SCRIPT_DIR/dist/style.css")
  printf '\n(function(){var s=document.createElement("style");s.textContent=%s;document.head.appendChild(s)})();\n' \
    "$(python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))" < "$SCRIPT_DIR/dist/style.css")" \
    >> "$REPORT_DIR/bundle.js"
fi

echo "Bundle built and copied to $REPORT_DIR/bundle.js"
