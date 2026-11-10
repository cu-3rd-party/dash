// Long donation comments are clamped by CSS; the button appears only when text is actually cut.
function updateClamp(box: HTMLElement) {
  const more = box.parentElement?.querySelector<HTMLElement>('[data-clamp-more]');
  if (more && box.classList.contains('line-clamp-4')) {
    more.classList.toggle('hidden', box.scrollHeight <= box.clientHeight);
  }
}

function initClamp() {
  for (const box of document.querySelectorAll<HTMLElement>('[data-clamp]')) updateClamp(box);
}

document.addEventListener('click', (event) => {
  const button = (event.target as HTMLElement).closest<HTMLElement>('[data-clamp-more]');
  if (!button) return;
  button.previousElementSibling?.classList.remove('line-clamp-4');
  button.classList.add('hidden');
});

document.addEventListener('DOMContentLoaded', initClamp);
document.addEventListener('htmx:afterSwap', initClamp);
window.addEventListener('resize', initClamp);
