// Listens on document: forms come and go with page loads and htmx swaps.

// <form data-autosubmit>: filters apply as soon as a select changes
document.addEventListener('change', (event) => {
  const target = event.target as HTMLElement;
  const form = target.closest<HTMLFormElement>('form[data-autosubmit]');
  if (form && target instanceof HTMLSelectElement) form.requestSubmit();
});

// «Анонимно» disables the name field next to it
document.addEventListener('change', (event) => {
  const toggle = event.target as HTMLInputElement;
  if (!toggle.matches('[data-anon-toggle]')) return;
  const name = toggle.form?.querySelector<HTMLInputElement>('[data-anon-target]');
  if (name) name.disabled = toggle.checked;
});

// Donation amounts that are not accepted (mirrors money.Funny): the whole roubles
// must not land on one of the data-funny numbers.
export function isFunnyAmount(raw: string, funny: number[]): boolean {
  const value = parseFloat(raw.replace(/[\s_]/g, '').replace(',', '.'));
  if (!Number.isFinite(value) || value < 0) return false;
  return funny.includes(Math.floor(Math.round(value * 100) / 100));
}

document.addEventListener('input', (event) => {
  const input = event.target as HTMLInputElement;
  if (!input.matches?.('[data-funny]')) return;
  const funny = (input.dataset.funny ?? '').split(',').map(Number);
  input.setCustomValidity(isFunnyAmount(input.value, funny) ? 'Такая сумма не принимается, выберите другую.' : '');
});
