import './forms.ts';

test('the anonymous toggle disables the name field', () => {
  document.body.innerHTML = `<form>
    <input name="donor" data-anon-target>
    <input type="checkbox" name="anonymous" data-anon-toggle>
  </form>`;
  const toggle = document.querySelector<HTMLInputElement>('[data-anon-toggle]')!;
  const name = document.querySelector<HTMLInputElement>('[data-anon-target]')!;
  toggle.checked = true;
  toggle.dispatchEvent(new Event('change', {bubbles: true}));
  expect(name.disabled).toBe(true);
  toggle.checked = false;
  toggle.dispatchEvent(new Event('change', {bubbles: true}));
  expect(name.disabled).toBe(false);
});

test('a select in a data-autosubmit form submits it', () => {
  document.body.innerHTML = `<form data-autosubmit><select name="cat"><option>1</option></select></form>`;
  const form = document.querySelector('form')!;
  const submit = vi.fn();
  form.requestSubmit = submit;
  form.querySelector('select')!.dispatchEvent(new Event('change', {bubbles: true}));
  expect(submit).toHaveBeenCalledOnce();
});
