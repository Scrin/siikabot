import { readFileSync } from 'node:fs'
import { expect, test, type Page } from '@playwright/test'

// The dashboard in a browser, against the bot the Go end-to-end test TestDashboardInABrowser has set up

/** Someone the test made, with what it takes to send a message as their Matrix client would */
interface Person {
  userId: string
  /** Their Matrix access token */
  token: string
  /** A room they share with the bot, to send !auth in */
  roomId: string
}

/** What the Go test made: who is who, where everything is, and the texts the dashboard should show */
interface World {
  dashboard: string
  homeserver: string
  alice: Person
  bob: Person
  admin: Person
  texts: {
    /** Alice's reminder, set in the group room */
    reminder: string
    /** Alice's memories, saved in the group room and in her direct chat with the bot */
    memory: string
    dmMemory: string
    /** What the bot remembers about the group room */
    roomMemory: string
    roomName: string
    webhookName: string
    /** The model in the chat configuration */
    model: string
  }
}

const worldFile = process.env.SIIKABOT_E2E_WORLD
if (!worldFile) {
  throw new Error('Run these through the Go end-to-end test: go test -tags e2e ./e2e -run TestDashboardInABrowser')
}
const world: World = JSON.parse(readFileSync(worldFile, 'utf8'))

const AUTH_TOKEN_KEY = 'siikabot_auth_token'

/** Skips a test of a known open bug, unless SIIKABOT_KNOWN_BUGS is set, as testutil/knownbug does in Go */
function knownBug(finding: string, what: string) {
  test.skip(!process.env.SIIKABOT_KNOWN_BUGS, `known bug ${finding} (.scratch/review-2026-11-20.md): ${what}`)
}

/** A card, by its title. The Rooms section has a Rooms card in it, so the card is the last of the name. */
function card(page: Page, title: string) {
  return page.getByRole('region', { name: title, exact: true }).last()
}

/** Sends a message into a person's room, as their Matrix client would */
async function say(person: Person, body: string) {
  const txn = `e2e-${Date.now()}-${Math.random().toString(36).slice(2)}`
  const response = await fetch(
    `${world.homeserver}/_matrix/client/v3/rooms/${encodeURIComponent(person.roomId)}/send/m.room.message/${txn}`,
    {
      method: 'PUT',
      headers: { Authorization: `Bearer ${person.token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ msgtype: 'm.text', body }),
    },
  )
  expect(response.ok, `sending a message as ${person.userId} gave ${response.status}`).toBe(true)
}

/** Logs in on an open dashboard the way people do: the person sends the !auth command it shows to the bot */
async function logIn(page: Page, person: Person) {
  await page.getByRole('button', { name: 'Login with Matrix' }).click()
  const command = await page.locator('code').filter({ hasText: '!auth ' }).textContent()
  await say(person, command!.trim())
  await expect(card(page, 'Account').getByText(person.userId, { exact: true })).toBeVisible()
}

/** The session token the dashboard keeps */
function sessionToken(page: Page) {
  return page.evaluate((key) => localStorage.getItem(key), AUTH_TOKEN_KEY)
}

/** Whether the API still takes a session token */
async function sessionWorks(token: string) {
  const response = await fetch(`${world.dashboard}/api/auth/me`, { headers: { Authorization: `Bearer ${token}` } })
  return response.status !== 401
}

test.beforeEach(async ({ page }) => {
  // The WebGL background and the cursor trail add nothing to what's tested
  await page.addInitScript(() => localStorage.setItem('siikabot_ui_effects', 'minimal'))
  await page.goto(world.dashboard)
})

test("someone who logs in sees what's theirs, and none of the admin's cards", async ({ page }) => {
  await expect(page.getByText('OK', { exact: true }).first()).toBeVisible()
  await logIn(page, world.alice)

  const reminders = card(page, 'Reminders')
  await expect(reminders.getByText(world.texts.reminder)).toBeVisible()
  await expect(reminders.getByText(world.texts.roomName)).toBeVisible()
  const memories = card(page, 'Your memories')
  await expect(memories.getByText(world.texts.memory)).toBeVisible()
  await expect(memories.getByText(world.texts.dmMemory)).toBeVisible()

  // A room's stats open under it, with the people who talk the most
  const rooms = card(page, 'Rooms')
  await rooms.getByText(world.texts.roomName).click()
  const mostMessages = rooms.getByRole('list', { name: /most messages/i })
  await expect(mostMessages.getByText(world.bob.userId.slice(1).split(':')[0]).first()).toBeVisible()

  for (const title of ['Webhooks', 'Room memories', 'Usage', 'Configuration', 'Message activity']) {
    await expect(page.getByRole('region', { name: title, exact: true })).toHaveCount(0)
  }
})

test('the admin sees every card', async ({ page }) => {
  await logIn(page, world.admin)

  await expect(card(page, 'Webhooks').getByText(world.texts.webhookName)).toBeVisible()
  await expect(card(page, 'Room memories').getByText(world.texts.roomMemory)).toBeVisible()
  await expect(card(page, 'Usage').getByText(world.texts.roomName).first()).toBeVisible()
  await expect(card(page, 'Configuration').getByText(world.texts.model).first()).toBeVisible()
  await expect(card(page, 'Message activity').getByText(world.texts.roomName).first()).toBeVisible()
})

test('logging out ends the session, here and on the server', async ({ page }) => {
  await logIn(page, world.alice)
  const token = await sessionToken(page)

  await card(page, 'Account').getByRole('button', { name: 'Logout' }).click()
  await expect(page.getByRole('button', { name: 'Login with Matrix' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Reminders', exact: true })).toHaveCount(0)
  expect(await sessionToken(page)).toBeNull()
  // The dashboard tells the server without waiting for it
  await expect.poll(() => sessionWorks(token!)).toBe(false)
})

test('a session that ends elsewhere logs the dashboard out', async ({ page }) => {
  knownBug('B13', 'a dead session is never noticed, and every card shows an error instead')
  await logIn(page, world.alice)
  const token = await sessionToken(page)

  // The session ends without this page doing anything, as when the person logs out in another browser
  await fetch(`${world.dashboard}/api/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
  // The cards' next refresh, within 10 seconds, finds the session gone
  await expect(page.getByRole('button', { name: 'Login with Matrix' })).toBeVisible({ timeout: 30_000 })
})

test("someone who logs in after another in the same tab never sees the other's data", async ({ page }) => {
  knownBug('P10', "logging out leaves the previous person's data cached, and the next person is shown it")
  await logIn(page, world.alice)
  await expect(card(page, 'Reminders').getByText(world.texts.reminder)).toBeVisible()
  await expect(card(page, 'Your memories').getByText(world.texts.memory)).toBeVisible()
  await card(page, 'Account').getByRole('button', { name: 'Logout' }).click()

  // Bob's own lists are held back, so anything the cards show meanwhile can only be from before
  let release = () => {}
  const held = new Promise<void>((resolve) => (release = resolve))
  await page.route(/\/api\/(reminders|memories)$/, async (route) => {
    await held
    await route.continue()
  })
  try {
    await logIn(page, world.bob)
    await expect(card(page, 'Reminders')).toBeVisible()
    await expect(page.getByText(world.texts.reminder)).toHaveCount(0)
    await expect(page.getByText(world.texts.memory)).toHaveCount(0)
  } finally {
    release()
  }
})

// Last, since the others look for the reminder. The Go test checks that the bot no longer has it.
test('someone deletes a reminder', async ({ page }) => {
  await logIn(page, world.alice)
  const reminders = card(page, 'Reminders')
  await expect(reminders.getByText(world.texts.reminder)).toBeVisible()

  await reminders.getByRole('button', { name: 'Delete' }).click()
  await reminders.getByRole('button', { name: 'Confirm' }).click()
  await expect(reminders.getByText(world.texts.reminder)).toHaveCount(0)
})
