<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { Service as Halo } from '../bindings/halobattery'
import { Devices as DevicesEvent } from '../bindings/events'

const state = ref(null)
const error = ref('')
const refreshing = ref(false)
const settingsOpen = ref(false)
const now = ref(Date.now())
const editing = ref('')
const draft = ref('')
const nameInput = ref(null)

let off = null
let offSettings = null
let tick = null
let saveTimer = null

const cfg = computed(() => state.value?.config)
const devices = computed(() => state.value?.devices ?? [])
const hiddenDevices = computed(() => state.value?.hidden_devices ?? [])
const online = computed(() => devices.value.filter((d) => d.online).length)

const RING = 2 * Math.PI * 26

/* ---------- translations ---------- */

const dict = {
  en: {
    'header.online': '{n} online',
    'header.known': '{n} known',
    'header.updated': 'updated {ago}',
    'app.loading': 'Loading…',
    'ago.never': 'never',
    'ago.now': 'just now',
    'ago.s': '{n}s ago',
    'ago.min': '{n} min ago',
    'ago.h': '{n} h ago',
    'status.asleep': 'Asleep',
    'status.charging': 'Charging',
    'status.unknown': 'Level unknown',
    'status.battery': 'On battery',
    'empty.title': 'No device detected',
    'empty.body':
      'Wireless mice, keyboards and headsets show up here through their vendor protocol, or through UPower when the system already knows them.',
    'empty.hint': 'Check /dev/hidraw* permissions if a device should appear.',
    'action.settings': 'Settings',
    'action.refresh': 'Refresh',
    'action.rename': 'Rename',
    'action.hide': 'Hide',
    'action.close': 'Close',
    'action.quit': 'Quit',
    'time.hm': '{h} h {m:02d} left',
    'time.m': '{m} min left',
    'settings.title': 'Settings',
    'settings.general': 'General',
    'settings.interval': 'Refresh interval',
    'settings.interval_value': '{n}s',
    'settings.low': 'Low battery alert',
    'settings.notify': 'Desktop notifications',
    'settings.autostart': 'Start with the session',
    'settings.language': 'Interface language',
    'settings.lang_auto': 'Automatic',
    'settings.lang_en': 'English',
    'settings.lang_fr': 'Français',
    'settings.devices': 'Devices',
    'settings.alert_at': 'Alert at',
    'settings.hidden': 'Hidden devices',
    'settings.none_hidden': 'No hidden device',
    'settings.show': 'Show',
    'settings.sources': 'Sources',
  },
  fr: {
    'header.online': '{n} en ligne',
    'header.known': '{n} connu(s)',
    'header.updated': 'mis à jour {ago}',
    'app.loading': 'Chargement…',
    'ago.never': 'jamais',
    'ago.now': "à l'instant",
    'ago.s': 'il y a {n} s',
    'ago.min': 'il y a {n} min',
    'ago.h': 'il y a {n} h',
    'status.asleep': 'En veille',
    'status.charging': 'En charge',
    'status.unknown': 'Niveau inconnu',
    'status.battery': 'Sur batterie',
    'empty.title': 'Aucun appareil détecté',
    'empty.body':
      "Les souris, claviers et casques sans fil apparaissent ici via leur protocole constructeur, ou via UPower quand le système les connaît déjà.",
    'empty.hint': 'Vérifie les droits sur /dev/hidraw* si un appareil devrait apparaître.',
    'action.settings': 'Réglages',
    'action.refresh': 'Rafraîchir',
    'action.rename': 'Renommer',
    'action.hide': 'Masquer',
    'action.close': 'Fermer',
    'action.quit': 'Quitter',
    'time.hm': '{h} h {m:02d} restantes',
    'time.m': '{m} min restantes',
    'settings.title': 'Réglages',
    'settings.general': 'Général',
    'settings.interval': 'Intervalle de rafraîchissement',
    'settings.interval_value': '{n} s',
    'settings.low': 'Alerte batterie faible',
    'settings.notify': 'Notifications bureau',
    'settings.autostart': "Lancer avec la session",
    'settings.language': "Langue de l'interface",
    'settings.lang_auto': 'Automatique',
    'settings.lang_en': 'English',
    'settings.lang_fr': 'Français',
    'settings.devices': 'Appareils',
    'settings.alert_at': 'Alerte à',
    'settings.hidden': 'Appareils masqués',
    'settings.none_hidden': 'Aucun appareil masqué',
    'settings.show': 'Afficher',
    'settings.sources': 'Sources',
  },
}

const lang = computed(() => {
  const want = cfg.value?.language ?? 'auto'
  if (want !== 'auto') return want
  const nav = (navigator.language || 'en').slice(0, 2).toLowerCase()
  return nav === 'fr' ? 'fr' : 'en'
})

function t(key, vars) {
  const table = dict[lang.value] ?? dict.en
  const raw = table[key] ?? dict.en[key] ?? key
  if (!vars) return raw
  return raw.replace(/\{(\w+)(?::(\w+))?\}/g, (_, name, fmt) => {
    const v = vars[name]
    if (v == null) return ''
    return fmt === '02d' ? String(v).padStart(2, '0') : String(v)
  })
}

/* ---------- rendering helpers ---------- */

function colour(d) {
  if (!d.online || d.level == null) return 'var(--muted)'
  const low = cfg.value?.alerts?.[d.key] ?? cfg.value?.low ?? 20
  if (d.charging || d.level > low + 20) return 'var(--green)'
  if (d.level > low) return 'var(--amber)'
  return 'var(--red)'
}

function dash(d) {
  const pct = d.level == null ? 0 : Math.min(100, Math.max(0, d.level))
  return `${(RING * pct) / 100} ${RING}`
}

function status(d) {
  if (!d.online) return t('status.asleep')
  if (d.charging) return t('status.charging')
  if (d.level == null) return t('status.unknown')
  return t('status.battery')
}

function label(d) {
  if (d.level == null) return '--'
  return `${d.level}%`
}

function kind(d) {
  return d.kind || d.source || ''
}

function ago(iso) {
  if (!iso) return t('ago.never')
  const then = new Date(iso).getTime()
  if (!then) return t('ago.never')
  const s = Math.max(0, Math.round((now.value - then) / 1000))
  if (s < 5) return t('ago.now')
  if (s < 60) return t('ago.s', { n: s })
  if (s < 3600) return t('ago.min', { n: Math.round(s / 60) })
  return t('ago.h', { n: Math.round(s / 3600) })
}

function spark(levels) {
  if (!levels || levels.length < 2) return ''
  const w = 100
  const h = 22
  const min = Math.min(...levels)
  const max = Math.max(...levels)
  const span = Math.max(1, max - min)
  return levels
    .map((v, i) => `${((i / (levels.length - 1)) * w).toFixed(1)},${(h - ((v - min) / span) * (h - 2) - 1).toFixed(1)}`)
    .join(' ')
}

function trend(key) {
  return state.value?.history?.[key]
}

function remaining(key) {
  const minutes = trend(key)?.minutes
  if (minutes == null || minutes < 0) return ''
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return h > 0 ? t('time.hm', { h, m }) : t('time.m', { m })
}

/* ---------- config ---------- */

function payload() {
  const c = cfg.value ?? {}
  return {
    interval: Number(c.interval) || 60,
    low: Number(c.low) || 0,
    notify: !!c.notify,
    autostart: !!c.autostart,
    disabled_providers: c.disabled_providers ?? [],
    names: c.names ?? {},
    hidden: c.hidden ?? [],
    alerts: c.alerts ?? {},
    language: c.language ?? 'auto',
  }
}

async function load() {
  try {
    state.value = await Halo.GetState()
    error.value = ''
  } catch (e) {
    error.value = String(e)
  }
}

async function refresh() {
  refreshing.value = true
  try {
    await Halo.RefreshNow()
    setTimeout(load, 1200)
  } finally {
    refreshing.value = false
  }
}

function queueSave() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    if (!cfg.value) return
    try {
      await Halo.SaveConfig(payload())
      error.value = ''
      await load()
    } catch (e) {
      error.value = String(e)
    }
  }, 350)
}

function providerEnabled(name) {
  return !(cfg.value?.disabled_providers ?? []).includes(name)
}

function toggleProvider(name, enabled) {
  const disabled = new Set(cfg.value?.disabled_providers ?? [])
  if (enabled) disabled.delete(name)
  else disabled.add(name)
  cfg.value.disabled_providers = [...disabled]
  queueSave()
}

function alertFor(key) {
  return cfg.value?.alerts?.[key] ?? cfg.value?.low ?? 20
}

function setAlert(key, value) {
  const alerts = { ...(cfg.value.alerts ?? {}) }
  const n = Number(value)
  if (!Number.isFinite(n) || n === cfg.value.low) delete alerts[key]
  else alerts[key] = Math.max(0, Math.min(100, Math.round(n)))
  cfg.value.alerts = alerts
  queueSave()
}

function isHidden(key) {
  return (cfg.value?.hidden ?? []).includes(key)
}

function setHidden(key, hidden) {
  const set = new Set(cfg.value?.hidden ?? [])
  if (hidden) set.add(key)
  else set.delete(key)
  cfg.value.hidden = [...set]
  queueSave()
}

async function startRename(d) {
  editing.value = d.key
  draft.value = d.name
  await nextTick()
  const el = Array.isArray(nameInput.value) ? nameInput.value[0] : nameInput.value
  el?.focus?.()
  el?.select?.()
}

function saveRename() {
  if (!editing.value) return
  const key = editing.value
  const name = draft.value.trim()
  const names = { ...(cfg.value.names ?? {}) }
  if (name) names[key] = name
  else delete names[key]
  cfg.value.names = names
  editing.value = ''
  queueSave()
}

function cancelRename() {
  editing.value = ''
}

function onKey(e) {
  if (e.key !== 'Escape') return
  if (editing.value) {
    cancelRename()
    return
  }
  settingsOpen.value = false
}

onMounted(() => {
  load()
  off = Events.On(DevicesEvent, () => {
    load() // also refreshes the provider notes and trends
  })
  offSettings = Events.On('open-settings', () => {
    settingsOpen.value = true
  })
  tick = setInterval(() => (now.value = Date.now()), 15000)
  window.addEventListener('keydown', onKey)
})

onUnmounted(() => {
  off?.()
  offSettings?.()
  clearInterval(tick)
  clearTimeout(saveTimer)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <main v-if="state" class="shell">
    <header class="top">
      <div>
        <h1>HaloBattery</h1>
        <p class="sub">
          {{ t('header.online', { n: online }) }} · {{ t('header.known', { n: devices.length }) }} ·
          {{ t('header.updated', { ago: ago(state.updated) }) }}
        </p>
      </div>
      <div class="actions">
        <button class="icon" :title="t('action.settings')" :aria-label="t('action.settings')" @click="settingsOpen = true">
          ⚙
        </button>
        <button
          class="icon"
          :class="{ spin: refreshing }"
          :title="t('action.refresh')"
          :aria-label="t('action.refresh')"
          @click="refresh"
        >
          ⟳
        </button>
      </div>
    </header>

    <p v-if="error" class="error">{{ error }}</p>

    <section v-if="devices.length" class="devices">
      <article v-for="d in devices" :key="d.key" class="card" :class="{ asleep: !d.online }">
        <svg class="ring" viewBox="0 0 64 64" aria-hidden="true">
          <circle class="track" cx="32" cy="32" r="26" />
          <circle
            class="fill"
            cx="32"
            cy="32"
            r="26"
            :stroke="colour(d)"
            :stroke-dasharray="dash(d)"
          />
        </svg>
        <div class="ring-label" :style="{ color: colour(d) }">{{ label(d) }}</div>
        <div class="meta">
          <div class="name-row">
            <input
              v-if="editing === d.key"
              ref="nameInput"
              v-model="draft"
              class="rename"
              maxlength="40"
              @keydown.enter.prevent="saveRename"
              @keydown.esc.prevent="cancelRename"
              @blur="saveRename"
            />
            <h2 v-else :title="d.name" @dblclick="startRename(d)">{{ d.name }}</h2>
            <button
              v-if="editing !== d.key"
              class="mini"
              :title="t('action.rename')"
              :aria-label="t('action.rename')"
              @click="startRename(d)"
            >
              ✎
            </button>
            <button
              class="mini"
              :title="t('action.hide')"
              :aria-label="t('action.hide')"
              @click="setHidden(d.key, true)"
            >
              ✕
            </button>
          </div>
          <p class="status" :class="{ charging: d.charging && d.online }">
            <span class="dot" :style="{ background: colour(d) }"></span>
            {{ status(d) }}<template v-if="d.approx"> · {{ d.approx }}</template>
          </p>
          <p class="tags">
            <span v-if="kind(d)" class="tag">{{ kind(d) }}</span>
            <span v-if="d.addr" class="tag mono">{{ d.addr.toUpperCase() }}</span>
            <span class="tag">{{ d.source }}</span>
          </p>
          <div v-if="trend(d.key)?.levels?.length > 1" class="trend">
            <svg viewBox="0 0 100 22" preserveAspectRatio="none" aria-hidden="true">
              <polyline :points="spark(trend(d.key).levels)" :stroke="colour(d)" />
            </svg>
            <span v-if="remaining(d.key)" class="left">{{ remaining(d.key) }}</span>
          </div>
        </div>
      </article>
    </section>

    <section v-else class="empty">
      <h2>{{ t('empty.title') }}</h2>
      <p>{{ t('empty.body') }}</p>
      <p class="hint">{{ t('empty.hint') }}</p>
    </section>

    <section v-if="state.notes?.length" class="notes">
      <p v-for="note in state.notes" :key="note">{{ note }}</p>
    </section>

    <div v-if="settingsOpen" class="modal" @mousedown.self="settingsOpen = false">
      <div class="sheet" role="dialog" aria-modal="true" aria-labelledby="settings-title">
        <header class="sheet-head">
          <h2 id="settings-title">{{ t('settings.title') }}</h2>
          <button class="icon" :title="t('action.close')" :aria-label="t('action.close')" @click="settingsOpen = false">
            ✕
          </button>
        </header>

        <div class="settings">
          <p class="group-title">{{ t('settings.general') }}</p>

          <label class="row">
            <span>{{ t('settings.interval') }} <em>{{ t('settings.interval_value', { n: cfg.interval }) }}</em></span>
            <input v-model.number="cfg.interval" type="range" min="5" max="600" step="5" @change="queueSave" />
          </label>

          <label class="row">
            <span>{{ t('settings.low') }} <em>{{ cfg.low }}%</em></span>
            <input v-model.number="cfg.low" type="range" min="0" max="80" step="1" @change="queueSave" />
          </label>

          <label class="row inline">
            <span>{{ t('settings.notify') }}</span>
            <input v-model="cfg.notify" type="checkbox" @change="queueSave" />
          </label>

          <label class="row inline">
            <span>{{ t('settings.autostart') }}</span>
            <input v-model="cfg.autostart" type="checkbox" @change="queueSave" />
          </label>

          <label class="row inline">
            <span>{{ t('settings.language') }}</span>
            <select v-model="cfg.language" @change="queueSave">
              <option value="auto">{{ t('settings.lang_auto') }}</option>
              <option value="en">{{ t('settings.lang_en') }}</option>
              <option value="fr">{{ t('settings.lang_fr') }}</option>
            </select>
          </label>

          <div v-if="devices.length" class="providers">
            <p class="group-title">{{ t('settings.devices') }}</p>
            <label v-for="d in devices" :key="d.key" class="row inline">
              <span>{{ d.name }}</span>
              <span class="alert-input">
                {{ t('settings.alert_at') }}
                <input
                  type="number"
                  min="0"
                  max="100"
                  step="1"
                  :value="alertFor(d.key)"
                  @change="setAlert(d.key, $event.target.value)"
                />
                %
              </span>
            </label>
          </div>

          <div class="providers">
            <p class="group-title">{{ t('settings.hidden') }}</p>
            <p v-if="!hiddenDevices.length" class="muted-line">{{ t('settings.none_hidden') }}</p>
            <label v-for="d in hiddenDevices" :key="d.key" class="row inline">
              <span>{{ d.name }}</span>
              <button class="ghost" @click="setHidden(d.key, false)">{{ t('settings.show') }}</button>
            </label>
          </div>

          <div v-if="state.providers.length" class="providers">
            <p class="group-title">{{ t('settings.sources') }}</p>
            <label v-for="p in state.providers" :key="p.name" class="row inline">
              <span>{{ p.name }}</span>
              <input
                type="checkbox"
                :checked="providerEnabled(p.name)"
                @change="toggleProvider(p.name, $event.target.checked)"
              />
            </label>
          </div>
        </div>
      </div>
    </div>

    <footer>
      <span>HaloBattery {{ state.version }} · {{ state.platform }}</span>
      <button class="ghost" @click="Halo.Quit()">{{ t('action.quit') }}</button>
    </footer>
  </main>

  <main v-else class="shell loading">
    <p>{{ t('app.loading') }}</p>
  </main>
</template>
