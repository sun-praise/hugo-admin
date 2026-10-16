// Package analytics pins the Umami tracker contract shipped in the admin SPA
// (frontend/src/analytics.ts, wired in main.tsx) via cross-language literal
// tests — the tracker itself lives in the frontend, this package only guards
// it from Go side.
package analytics
