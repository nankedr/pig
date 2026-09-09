import { pathToFileURL } from 'node:url';
export async function openBrowser() {
  const { chromium } = await import(process.env.PIG_PLAYWRIGHT_MODULE || 'playwright');
  return chromium.launch({headless:true, ...(process.env.PIG_CHROMIUM ? {executablePath:process.env.PIG_CHROMIUM} : {})});
}
export async function observeExport(path) {
  const browser = await openBrowser();
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto(pathToFileURL(path).href);
    await page.locator('#messages .user-message').first().waitFor();
    const result = await page.evaluate(() => ({
      heading: document.querySelector('#messages h1')?.textContent,
      bold: document.querySelector('#messages strong')?.textContent,
      code: document.querySelector('#messages pre code')?.textContent,
      highlighted: !!document.querySelector('#messages pre code span'),
      table: document.querySelector('#messages table')?.textContent.replace(/\s/g,''),
      toolText: document.querySelector('.tool-output')?.textContent.trim(),
      compaction: document.querySelector('.compaction-content')?.textContent.trim(),
      summary: document.querySelector('.branch-summary .markdown-content')?.textContent.trim(),
      visibleIDs: [...document.querySelectorAll('#messages [id^="entry-"]')].map(x=>x.id),
      siblingHidden: !document.querySelector('#messages')?.textContent.includes('discarded sibling'),
    }));
    await page.locator('[data-filter="all"]').click();
    await page.locator('.tree-node').filter({hasText:'discarded sibling'}).click();
    result.siblingNavigable = (await page.locator('#messages').textContent()).includes('discarded sibling');
    if(errors.length) throw Error(errors.join('\n'));
    return result;
  } finally {await browser.close();}
}
