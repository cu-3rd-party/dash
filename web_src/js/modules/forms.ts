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
