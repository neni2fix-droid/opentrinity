"""Apply local compatibility patches to LiteLLM's Responses API transformation.

LiteLLM 1.74.15 validates upstream Responses API payloads with pydantic models
that mark some fields as required. OpenRouter (and other OpenAI-compatible
providers) legitimately omit those fields, so LiteLLM raises:

    1 validation error for ResponsesAPIResponse
    user
      Field required [type=missing]

That failure surfaces as a 502/408 from the gateway on POST /v1/responses.

This script patches both affected paths:

  * streaming  - litellm/llms/openai/responses/transformation.py
  * non-streaming - the same file, transform_response_api_response()

It is idempotent: each patch is guarded by its own marker comment, so running
it twice leaves the file unchanged.

Usage:
    python scripts/phase0/patch-litellm-responses.py <path-to-venv>
    python scripts/phase0/patch-litellm-responses.py C:\\path\\venv2
"""

import os
import sys
from pathlib import Path

REL = Path("Lib") / "site-packages" / "litellm" / "llms" / "openai" / "responses" / "transformation.py"

STREAM_MARKER = 'response_obj.setdefault("user", None)'
RESP_MARKER = "# Local compatibility patch (responses):"

STREAM_ANCHOR = "        return event_pydantic_model(**parsed_chunk)"

STREAM_PATCH = """        # Local compatibility patch (streaming): OpenRouter/OpenAI-compatible
        # streams may omit fields that LiteLLM 1.74.15's Responses event
        # models mark as required.
        response_obj = parsed_chunk.get("response")
        if isinstance(response_obj, dict):
            response_obj.setdefault("user", None)
            response_obj.setdefault("metadata", {})
        return event_pydantic_model(**parsed_chunk)"""

RESP_ANCHOR = "        return ResponsesAPIResponse(**raw_response_json)"

RESP_PATCH = """        # Local compatibility patch (responses): OpenRouter/OpenAI-compatible
        # providers may omit fields that LiteLLM 1.74.15's Responses response
        # model marks as required.
        raw_response_json.setdefault("user", None)
        raw_response_json.setdefault("metadata", {})
        return ResponsesAPIResponse(**raw_response_json)"""


def patch(text: str, marker: str, anchor: str, replacement: str) -> tuple:
    if marker in text:
        return text, False
    if anchor not in text:
        raise SystemExit("anchor not found, upstream layout changed: " + anchor.strip())
    return text.replace(anchor, replacement), True


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2

    target = Path(sys.argv[1]) / REL
    if not target.is_file():
        raise SystemExit("target not found: " + str(target))

    original = target.read_text(encoding="utf-8")
    text, stream_applied = patch(original, STREAM_MARKER, STREAM_ANCHOR, STREAM_PATCH)
    text, resp_applied = patch(text, RESP_MARKER, RESP_ANCHOR, RESP_PATCH)

    if text == original:
        print("no change needed, both patches already applied: " + str(target))
        return 0

    tmp = target.with_suffix(".py.otpatch.tmp")
    tmp.write_text(text, encoding="utf-8")
    os.replace(str(tmp), str(target))
    print("patched: " + str(target))
    print("  streaming    : " + ("applied" if stream_applied else "already present"))
    print("  non-streaming: " + ("applied" if resp_applied else "already present"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
