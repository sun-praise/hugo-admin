# Change: Fix AI Chat Send Button State After Response

## Why
Testing revealed that the AI chat send button remains disabled after the AI finishes responding. This is because the SSE stream may not be properly closing, preventing the `isLoading` state from being reset to `false` in the finally block.

## What Changes
- Fix the SSE stream handling to ensure the finally block executes when stream ends
- Add explicit isLoading = false when [DONE] marker is received as a fallback

## Impact
- Affected specs: ai-ui-chatbox
- Affected code:
  - `templates/base.html`: AI chat sendMessage function (lines 308-374)
