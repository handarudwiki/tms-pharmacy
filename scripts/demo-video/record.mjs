// Usage: CHROME_PATH=... node record.mjs ../../docs/demo.mp4  (backend on :8090, web dev server on :5174, freshly seeded data)
// Records a scripted walkthrough of the WMS and encodes it to MP4 with Chrome's MediaRecorder.
import puppeteer from 'puppeteer-core'
import fs from 'node:fs'

const OUT = process.argv[2]
const BASE = 'http://localhost:5174'
const W = 1280, H = 800
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))


const browser = await puppeteer.launch({ executablePath: process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true, protocolTimeout: 0 })
const page = await browser.newPage()
await page.setViewport({ width: W, height: H, deviceScaleFactor: 1 })

// Overlay: visible cursor, click ripple and caption bar (re-created on every document).
await page.evaluateOnNewDocument(() => {
  window.addEventListener('DOMContentLoaded', () => {
    const style = document.createElement('style')
    style.textContent = `
      #demo-cursor{position:fixed;z-index:99999;width:22px;height:22px;pointer-events:none;left:640px;top:400px;
        transform:translate(-3px,-2px);filter:drop-shadow(0 2px 3px rgba(0,0,0,.35));transition:transform .1s}
      #demo-cursor.down{transform:translate(-3px,-2px) scale(.85)}
      .demo-ripple{position:fixed;z-index:99998;width:36px;height:36px;margin:-18px 0 0 -18px;border-radius:50%;
        background:rgba(13,148,136,.35);pointer-events:none;animation:demo-r .5s ease-out forwards}
      @keyframes demo-r{from{transform:scale(.3);opacity:1}to{transform:scale(1.6);opacity:0}}
      #demo-caption{position:fixed;z-index:99997;left:50%;bottom:22px;transform:translateX(-50%);max-width:880px;
        background:rgba(17,19,30,.88);color:#fff;font:600 17px/1.4 'Plus Jakarta Sans',system-ui,sans-serif;
        padding:12px 22px;border-radius:14px;box-shadow:0 10px 30px rgba(0,0,0,.25);transition:opacity .3s;text-align:center;pointer-events:none}
      #demo-caption:empty{opacity:0}
      #demo-caption small{display:block;font-weight:500;font-size:12.5px;opacity:.75;margin-bottom:2px;letter-spacing:.04em}`
    document.head.appendChild(style)
    const c = document.createElement('div')
    c.id = 'demo-cursor'
    c.innerHTML = '<svg viewBox="0 0 24 24" width="22" height="22"><path d="M3 2l7 19 2.6-7.4L20 11z" fill="#111" stroke="#fff" stroke-width="1.6" stroke-linejoin="round"/></svg>'
    document.body.appendChild(c)
    const cap = document.createElement('div')
    cap.id = 'demo-caption'
    document.body.appendChild(cap)
    const last = JSON.parse(sessionStorage.getItem('demo-pos') || 'null')
    if (last) { c.style.left = last[0] + 'px'; c.style.top = last[1] + 'px' }
    cap.innerHTML = sessionStorage.getItem('demo-caption') || ''
    document.addEventListener('mousemove', (e) => {
      c.style.left = e.clientX + 'px'; c.style.top = e.clientY + 'px'
      sessionStorage.setItem('demo-pos', JSON.stringify([e.clientX, e.clientY]))
    }, true)
    document.addEventListener('mousedown', (e) => {
      c.classList.add('down')
      const r = document.createElement('div'); r.className = 'demo-ripple'
      r.style.left = e.clientX + 'px'; r.style.top = e.clientY + 'px'
      document.body.appendChild(r); setTimeout(() => r.remove(), 600)
    }, true)
    document.addEventListener('mouseup', () => c.classList.remove('down'), true)
  })
})

// --- screencast capture ---
const frames = []
const cdp = await page.createCDPSession()
cdp.on('Page.screencastFrame', async ({ data, metadata, sessionId }) => {
  frames.push({ data, t: metadata.timestamp })
  await cdp.send('Page.screencastFrameAck', { sessionId }).catch(() => {})
})

// --- helpers ---
let mouse = [W / 2, H / 2]
async function caption(step, text) {
  await page.evaluate((html) => {
    sessionStorage.setItem('demo-caption', html)
    const el = document.getElementById('demo-caption'); if (el) el.innerHTML = html
  }, step ? `<small>${step}</small>${text}` : text)
}
async function moveTo(x, y) {
  await page.mouse.move(x, y, { steps: Math.max(12, Math.round(Math.hypot(x - mouse[0], y - mouse[1]) / 18)) })
  mouse = [x, y]
}
async function center(h) {
  await h.evaluate((el) => el.scrollIntoView({ block: 'center', behavior: 'instant' }))
  const b = await h.boundingBox()
  return [b.x + b.width / 2, b.y + b.height / 2]
}
async function click(h, pause = 350) {
  let [x, y] = await center(h)
    await moveTo(x, y)
  await sleep(180)
  // The layout may still settle (maps, toasts); re-aim at the element's current position.
  const b = await h.boundingBox()
  if (b && (Math.abs(b.x + b.width / 2 - x) > 3 || Math.abs(b.y + b.height / 2 - y) > 3)) {
    ;[x, y] = [b.x + b.width / 2, b.y + b.height / 2]
    await moveTo(x, y)
    await sleep(120)
  }
  await page.mouse.down(); await sleep(70); await page.mouse.up()
  await sleep(pause)
}
// Last visible element (modals render at the end of <body>) whose text contains `text`.
async function byText(text, sel = 'button, a') {
  await page.waitForFunction((t, s) => [...document.querySelectorAll(s)].some((e) => e.offsetParent && e.textContent.includes(t)), { timeout: 10000 }, text, sel)
  return page.evaluateHandle((t, s) => [...document.querySelectorAll(s)].filter((e) => e.offsetParent && e.textContent.includes(t)).pop(), text, sel)
}
// First visible match (e.g. the next stop's buttons in the driver app).
async function firstText(text, sel = 'button') {
  await page.waitForFunction((t, s) => [...document.querySelectorAll(s)].some((e) => e.offsetParent && e.textContent.trim() === t), { timeout: 10000 }, text, sel)
  return page.evaluateHandle((t, s) => [...document.querySelectorAll(s)].find((e) => e.offsetParent && e.textContent.trim() === t), text, sel)
}
async function within(rowText, rowSel, btnText) {
  return page.evaluateHandle((rt, rs, bt) => {
    const row = [...document.querySelectorAll(rs)].find((r) => r.textContent.includes(rt))
    return [...row.querySelectorAll('button')].find((b) => b.textContent.includes(bt))
  }, rowText, rowSel, btnText)
}
async function field(label) {
  return page.evaluateHandle((l) => {
    const scope = [...document.querySelectorAll('[role=dialog]')].pop() || document
    const lab = [...scope.querySelectorAll('label, .label')].find((e) => e.textContent.trim().startsWith(l))
    return lab.parentElement.querySelector('input, select, textarea')
  }, label)
}
async function type(h, text) {
  await click(h, 120)
  await h.evaluate((el) => el.select && el.select())
  await page.keyboard.type(String(text), { delay: 45 })
  await sleep(250)
}
async function setValue(h, value) {
  await click(h, 150)
  await h.evaluate((el, v) => { el.value = v; el.dispatchEvent(new Event('input', { bubbles: true })); el.dispatchEvent(new Event('change', { bubbles: true })) }, value)
  await sleep(300)
}
async function go(path) {
  await page.evaluate((p) => document.querySelector('#app').__vue_app__.config.globalProperties.$router.push(p), path)
  await sleep(1200)
}
async function scroll(y) {
  const from = await page.evaluate(() => window.scrollY)
  const steps = 40
  for (let i = 1; i <= steps; i++) {
    const e = 0.5 - Math.cos((Math.PI * i) / steps) / 2
    await page.evaluate((v) => window.scrollTo(0, v), from + (y - from) * e)
    await sleep(25)
  }
  await sleep(500)
}
const date = (d) => new Date(Date.now() + d * 864e5).toISOString().slice(0, 10)

// --- storyboard ---
async function nav(url) {
  for (let attempt = 1; ; attempt++) {
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 20000 })
      await page.waitForSelector('#email', { timeout: 20000 })
      return
    } catch (e) {
      if (attempt >= 4) throw e
      console.log('navigation retry', attempt)
    }
  }
}
await nav(BASE + '/login')
await page.evaluate(() => { localStorage.clear(); localStorage.setItem('tms.theme', 'light'); sessionStorage.clear() })
await nav(BASE + '/login')
await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 82, maxWidth: W, maxHeight: H, everyNthFrame: 1 })
await sleep(300)

async function signIn(email, step, text) {
  await caption(step, text)
  await type(await field('Email'), email)
  await type(await field('Password'), 'password123')
  await click(await byText('Sign in'), 2200)
}
// Sign in through a demo-account chip on the login page (fills the form, no typing).
async function signInChip(chip, step, text) {
  await caption(step, text)
  await click(await byText(chip), 700)
  await click(await byText('Sign in'), 2200)
}
async function signOut() {
  await click(await page.evaluateHandle(() => document.querySelector('button[aria-label="Sign out"]')), 1200)
}

async function story() {
await caption('PHARMACY TMS · DEMO', 'Sistem manajemen transportasi untuk distribusi farmasi — pendamping Pharmacy WMS')
await sleep(2600)
await signIn('transport@tms.local', '1 · LOGIN', 'Masuk sebagai Transport Manager')

await caption('2 · DASHBOARD', 'Rute hari ini di peta: trip terencana (putus-putus), trip berjalan & posisi kendaraan')
await moveTo(400, 520); await sleep(1200)
const tripCards = await page.$$('main li.cursor-pointer')
for (const c of tripCards.slice(0, 3)) { const [x, y] = await center(c); await moveTo(x, y); await sleep(1100) }
await caption('2 · DASHBOARD', 'Tren pengiriman, pipeline status, kepatuhan cold chain, exception & biaya ekspedisi')
await scroll(820); await sleep(2000)
await scroll(1500); await sleep(1800)
await scroll(0)

await caption('3 · INTEGRASI WMS', 'Order yang sudah di-packing di WMS otomatis masuk sebagai shipment — tanpa duplikasi')
await go('/integrations')
await click(await byText('Sync from WMS'), 2800)

await caption('4 · PLANNING', 'Auto-plan: cek kapasitas, kendaraan berpendingin, sertifikasi obat terkontrol & SIM driver')
await go('/planning')
await sleep(1200)
await click(await byText('Auto-plan'), 3000)
const hasTender = await page.evaluate(() => [...document.querySelectorAll('a')].some((a) => a.textContent.includes('Tender these')))
if (hasTender) {
  await caption('4 · PLANNING', 'Shipment yang tidak muat di armada sendiri ditampilkan beserta alasannya')
  await moveTo(560, 200); await sleep(2400)
  await caption('5 · TENDER EKSPEDISI', 'Sisa shipment ditenderkan ke ekspedisi — penawaran dihitung dari rate card')
  await click(await byText('Tender these'), 2000)
  await click(await byText('Open tender'), 2600)
  await caption('5 · TENDER EKSPEDISI', 'Penawaran yang tidak memenuhi syarat (tanpa cold chain) otomatis ditandai & tidak bisa menang')
  await moveTo(640, 520); await sleep(2400)
  const award = await page.evaluateHandle(() => [...document.querySelectorAll('[role=dialog] button')].find((b) => b.textContent.trim() === 'Award'))
  if (award.asElement()) await click(award, 2400)
  await page.keyboard.press('Escape'); await sleep(500)
}

await caption('6 · TRIP', 'Urutan stop dioptimalkan (nearest-neighbour + 2-opt) dengan ETA per jendela waktu')
await go('/trips')
await sleep(1000)
await click(await page.evaluateHandle(() => [...document.querySelectorAll('tbody tr')].find((r) => r.textContent.includes('Planned'))), 2400)
await moveTo(400, 300); await sleep(1600)
await click(await byText('Re-optimize'), 1800)
await caption('6 · TRIP', 'Rilis trip ke aplikasi driver — shipment berstatus Dispatched')
await click(await byText('Release to driver'), 2200)
await page.keyboard.press('Escape'); await sleep(500)

await signOut()
await signInChip('Driver', '7 · DRIVER APP', 'Driver Andi Wijaya membuka aplikasi — trip-nya sudah dirilis dispatcher')
await caption('7 · DRIVER APP', 'Mulai trip, konfirmasi tiba, lalu bukti serah terima (nama penerima + tanda tangan)')
await sleep(1500) // let the map settle
await click(await byText('Start trip'), 1800)
await page.waitForFunction(() => [...document.querySelectorAll('button')].some((b) => b.textContent.includes('Arrived')), { timeout: 10000 })
await click(await firstText('Arrived'), 1500)
await click(await firstText('Deliver'), 1000)
await type(await field("Receiver's name"), 'Sri Wahyuni (apoteker)')
await click(await byText('Receiver signed', 'label'), 600)
await type(await field('Note'), 'Segel utuh, 3 koli')
await click(await byText('Confirm delivery'), 2200)
await caption('7 · DRIVER APP', 'Lokasi kendaraan dikirim berkala untuk pelacakan')
await click(await byText('Send location'), 2000)
await signOut()

await signInChip('Transport manager', '8 · LIVE TRACKING', 'Kembali sebagai Transport Manager')
await caption('8 · LIVE TRACKING', 'Semua kendaraan di jalan, stop berikutnya & suhu reefer secara real-time')
await go('/tracking')
await sleep(2800)
await caption('9 · EXCEPTIONS', 'Ekskursi suhu di luar 2–8 °C, gagal kirim & keterlambatan tercatat sampai diselesaikan')
await go('/exceptions')
await sleep(2600)
await caption('10 · ANALYTICS', 'Jarak, biaya per km & per drop, utilisasi kendaraan, alasan gagal kirim, biaya ekspedisi')
await go('/analytics')
await sleep(1500)
await scroll(700); await sleep(1800)
await scroll(0)
await caption('', 'Mode gelap tersedia')
await go('/')
await click(await page.$('button[aria-label="Dark mode"]'), 2600)
await caption('PHARMACY TMS', 'Go microservices · gRPC · PostgreSQL · Vue 3 + Pinia · Leaflet')
await sleep(3000)

}
try {
  await story()
} catch (e) {
  await page.screenshot({ path: 'demo-failure.png' })
  throw e
}
await cdp.send('Page.stopScreencast')
console.log('frames', frames.length, 'duration', (frames.at(-1).t - frames[0].t).toFixed(1), 's')
await page.close()
fs.writeFileSync('frames.json', JSON.stringify(frames))

// --- encode: replay frames in real time onto a canvas recorded by MediaRecorder ---
const enc = await browser.newPage()
const out = fs.openSync(OUT, 'w')
await enc.exposeFunction('frame', (i) => frames[i] && { data: frames[i].data, t: frames[i].t })
await enc.exposeFunction('save', (b64) => { fs.writeSync(out, Buffer.from(b64, 'base64')) })
await enc.evaluate(async (n, W, H) => {
  const canvas = Object.assign(document.createElement('canvas'), { width: W, height: H })
  document.body.appendChild(canvas)
  const ctx = canvas.getContext('2d')
  const stream = canvas.captureStream(30)
  const rec = new MediaRecorder(stream, { mimeType: 'video/mp4;codecs=avc1.640028', videoBitsPerSecond: 2_500_000 })
  const pending = []
  rec.ondataavailable = (e) => pending.push(e.data.arrayBuffer().then((buf) => {
    let s = ''; const b = new Uint8Array(buf)
    for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000))
    return window.save(btoa(s))
  }))
  const load = async (i) => { const f = await window.frame(i); return { bmp: await createImageBitmap(await (await fetch('data:image/jpeg;base64,' + f.data)).blob()), t: f.t } }
  let cur = await load(0)
  ctx.drawImage(cur.bmp, 0, 0, W, H)
  rec.start()
  const start = performance.now(), t0 = cur.t
  for (let i = 1; i < n; i++) {
    const next = await load(i)
    const due = start + (next.t - t0) * 1000
    // keep repainting the current frame until the next one is due
    while (performance.now() < due) { ctx.drawImage(cur.bmp, 0, 0, W, H); await new Promise((r) => setTimeout(r, Math.min(33, due - performance.now()))) }
    cur = next
    ctx.drawImage(cur.bmp, 0, 0, W, H)
  }
  const endAt = performance.now() + 3500
  while (performance.now() < endAt) { ctx.drawImage(cur.bmp, 0, 0, W, H); await new Promise((r) => setTimeout(r, 33)) }
  await new Promise((r) => { rec.onstop = r; rec.stop() })
  await Promise.all(pending)
  // ensure sequential writes completed
}, frames.length, W, H)
fs.closeSync(out)
await browser.close()
console.log('saved', OUT, (fs.statSync(OUT).size / 1e6).toFixed(1), 'MB')
