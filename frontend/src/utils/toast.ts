let toastEl: HTMLDivElement | null = null;
let toastTimer: ReturnType<typeof setTimeout> | null = null;

/** 底部居中深色胶囊提示(对齐重设计原型的 Toast)。 */
export function toast(message: string): void {
  if (!toastEl || !document.body.contains(toastEl)) {
    toastEl = document.createElement('div');
    toastEl.className = 'app-toast';
    toastEl.setAttribute('role', 'status');
    document.body.appendChild(toastEl);
  }
  toastEl.textContent = message;
  toastEl.classList.add('is-visible');
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toastEl?.classList.remove('is-visible'), 2200);
}
