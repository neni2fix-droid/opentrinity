param(
    [Parameter(Mandatory = $true)]
    [string]$Venv
)

$ErrorActionPreference = 'Stop'

# Apply local compatibility patches to LiteLLM's Responses API transformation.
#
# LiteLLM 1.74.15 validates upstream Responses API payloads with pydantic
# models that mark some fields as required. OpenRouter (and other
# OpenAI-compatible providers) legitimately omit those fields, so LiteLLM raises:
#
#     1 validation error for ResponsesAPIResponse
#     user
#       Field required [type=missing]
#
# That failure surfaces as a 502/408 from the gateway on POST /v1/responses.
#
# Both affected paths are patched in
#   litellm/llms/openai/responses/transformation.py
#
#   * streaming     - transform_streaming_response()
#   * non-streaming - transform_response_api_response()
#
# Detection is based on the setdefault() calls themselves rather than on
# comment text, so the script is idempotent and does not double-apply a patch
# that was applied by hand in an earlier session.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/phase0/patch-litellm-responses.ps1 -Venv .phase0/venv2

$rel = 'Lib/site-packages/litellm/llms/openai/responses/transformation.py'
$target = Join-Path $Venv $rel

if (-not (Test-Path $target)) { throw "target not found: $target" }

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$nl = [Environment]::NewLine
$text = [System.IO.File]::ReadAllText($target)

$streamAnchor   = '        return event_pydantic_model(**parsed_chunk)'
$streamSentinel = 'response_obj.setdefault("user", None)'
$streamPatch = @(
    '        # Local compatibility patch (streaming): OpenRouter/OpenAI-compatible',
    '        # streams may omit fields that LiteLLM 1.74.15''s Responses event',
    '        # models mark as required.',
    '        response_obj = parsed_chunk.get("response")',
    '        if isinstance(response_obj, dict):',
    '            response_obj.setdefault("user", None)',
    '            response_obj.setdefault("metadata", {})',
    '        return event_pydantic_model(**parsed_chunk)'
) -join $nl

$respAnchor   = '        return ResponsesAPIResponse(**raw_response_json)'
$respSentinel = 'raw_response_json.setdefault("user", None)'
$respPatch = @(
    '        # Local compatibility patch (responses): OpenRouter/OpenAI-compatible',
    '        # providers may omit fields that LiteLLM 1.74.15''s Responses response',
    '        # model marks as required.',
    '        raw_response_json.setdefault("user", None)',
    '        raw_response_json.setdefault("metadata", {})',
    '        return ResponsesAPIResponse(**raw_response_json)'
) -join $nl

$streamApplied = $false
$respApplied   = $false

if ($text.Contains($streamSentinel)) {
    Write-Output '  streaming    : already present'
} else {
    if (-not $text.Contains($streamAnchor)) { throw "anchor not found for streaming, upstream layout changed" }
    $text = $text.Replace($streamAnchor, $streamPatch)
    $streamApplied = $true
    Write-Output '  streaming    : applied'
}

if ($text.Contains($respSentinel)) {
    Write-Output '  non-streaming: already present'
} else {
    if (-not $text.Contains($respAnchor)) { throw "anchor not found for non-streaming, upstream layout changed" }
    $text = $text.Replace($respAnchor, $respPatch)
    $respApplied = $true
    Write-Output '  non-streaming: applied'
}

if ($streamApplied -or $respApplied) {
    $tmp = "$target.otpatch.tmp"
    [System.IO.File]::WriteAllText($tmp, $text, $utf8NoBom)
    Move-Item -Path $tmp -Destination $target -Force
    Write-Output "patched: $target"
} else {
    Write-Output "no change needed, both patches already applied: $target"
}
