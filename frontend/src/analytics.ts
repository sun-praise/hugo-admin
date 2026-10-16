// Umami page-view analytics for the public demo deployment only.
//
// The tracker is injected AFTER the app mounts and ONLY when the page is
// served from the public demo host — local dev and private instances never
// contact the analytics origin at all (no request, no third-party code on
// the authenticated admin origin). The data-domains attribute is a second,
// tracker-side reporting guard (defense in depth), not a load gate.
const PUBLIC_HOST = 'hugo-admin.sun-praise.com'

export function initAnalytics(): void {
  if (location.hostname !== PUBLIC_HOST) return
  const script = document.createElement('script')
  script.src = 'https://analysis.sun-praise.com/script.js'
  script.setAttribute('data-website-id', '1922bd9a-6d58-4801-9bb1-f9cfe289139f')
  script.setAttribute('data-domains', PUBLIC_HOST)
  script.setAttribute('referrerpolicy', 'no-referrer')
  document.head.appendChild(script)
}
