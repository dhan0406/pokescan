#!/bin/sh
# Stand-in for the tesseract CLI, driven by environment variables set in tests.
[ -n "$FAKE_ARGS_OUT" ] && printf '%s\n' "$@" > "$FAKE_ARGS_OUT"
if [ -n "$FAKE_STDIN_OUT" ]; then cat > "$FAKE_STDIN_OUT"; else cat > /dev/null; fi
[ -n "$FAKE_SLEEP" ] && sleep "$FAKE_SLEEP"
[ -n "$FAKE_TSV" ] && cat "$FAKE_TSV"
exit "${FAKE_EXIT:-0}"
