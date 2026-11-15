<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { Service as Halo } from '../bindings/halobattery'
import { Devices as DevicesEvent } from '../bindings/events'

const state = ref(null)
const error = ref('')
const refreshing = ref(false)
const now = ref(Date.now())

let off = null
let tick = null
let saveTimer = null

const cfg = computed(() => state.value?.config)
const devices = computed(() => state.value?.devices ?? [])
const online = computed(() => devices.value.filter((d) => d.online).length)

const RING = 2 * Math.PI * 26

function colour(d) {
  if (!d.online || d.level == null) return 'var(--muted)'
  const low = cfg.value?.low ?? 20
  if (d.charging || d.level > low + 20) return 'var(--green)'
  if (d.level > low) return 'var(--amber)'
  return 'var(--red)'
}

function dash(d) {
  const pct = d.level == null ? 0 : Math.min(100, Math.max(0, d.level))
  return `${(RING * pct) / 100} ${RING}`
}

function status(d) {
  if (!d.online) return 'Asleep'
  if (d.charging) return 'Charging'
  if (d.level == null) return 'Level unknown'
  return 'On battery'
}

function label(d) {
  if (d.level == null) return '--'
  return `${d.level}%`
}

function kind(d) {
  return d.kind || d.source || ''
}

function ago(iso) {
  if (!iso) return 'never'
  const then = new Date(iso).getTime()
  if (!then) return 'never'
  const s = Math.max(0, Math.round((now.value - then) / 1000))
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.round(s / 60)} min ago`
  return `${Math.round(s / 3600)} h ago`
}

function providerEnabled(name) {
  return !(cfg.value?.disabled_providers ?? []).includes(name)
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
      await Halo.SaveConfig({
        interval: Number(cfg.value.interval) || 60,
        low: Number(cfg.value.low) || 0,
        notify: !!cfg.value.notify,
        autostart: !!cfg.value.autostart,
        disabled_providers: cfg.value.disabled_providers ?? [],
      })
      error.value = ''
      await load()
    } catch (e) {
      error.value = String(e)
    }
  }, 350)
}

function toggleProvider(name, enabled) {
  const disabled = new Set(cfg.value?.disabled_providers ?? [])
  if (enabled) disabled.delete(name)
  else disabled.add(name)
  cfg.value.disabled_providers = [...disabled]
  queueSave()
}

onMounted(() => {
  load()
  off = Events.On(DevicesEvent, () => {
    load() // also refreshes the provider notes
  })
  tick = setInterval(() => (now.value = Date.now()), 15000)
})

onUnmounted(() => {
  off?.()
  clearInterval(tick)
  clearTimeout(saveTimer)
})
</script>

<template>
  <main v-if="state" class="shell">
    <header class="top">
      <div>
        <h1>HaloBattery</h1>
        <p class="sub">
          {{ online }} online · {{ devices.length }} known · updated {{ ago(state.updated) }}
        </p>
      </div>
      <button class="icon" :class="{ spin: refreshing }" title="Refresh" @click="refresh">⟳</button>
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
          <h2 :title="d.name">{{ d.name }}</h2>
          <p class="status" :class="{ charging: d.charging && d.online }">
            <span class="dot" :style="{ background: colour(d) }"></span>
            {{ status(d) }}<template v-if="d.approx"> · {{ d.approx }}</template>
          </p>
          <p class="tags">
            <span v-if="kind(d)" class="tag">{{ kind(d) }}</span>
            <span v-if="d.addr" class="tag mono">{{ d.addr.toUpperCase() }}</span>
            <span class="tag">{{ d.source }}</span>
          </p>
        </div>
      </article>
    </section>

    <section v-else class="empty">
      <h2>No device detected</h2>
      <p>
        Wireless mice, keyboards and headsets show up here through their vendor
        protocol, or through UPower when the system already knows them.
      </p>
      <p class="hint">Check <code>/dev/hidraw*</code> permissions if a device should appear.</p>
    </section>

    <section v-if="state.notes?.length" class="notes">
      <p v-for="note in state.notes" :key="note">{{ note }}</p>
    </section>

    <section class="settings">
      <h2>Settings</h2>

      <label class="row">
        <span>Refresh interval <em>{{ cfg.interval }}s</em></span>
        <input v-model.number="cfg.interval" type="range" min="5" max="600" step="5" @change="queueSave" />
      </label>

      <label class="row">
        <span>Low battery alert <em>{{ cfg.low }}%</em></span>
        <input v-model.number="cfg.low" type="range" min="0" max="80" step="1" @change="queueSave" />
      </label>

      <label class="row inline">
        <span>Desktop notifications</span>
        <input v-model="cfg.notify" type="checkbox" @change="queueSave" />
      </label>

      <label class="row inline">
        <span>Start with the session</span>
        <input v-model="cfg.autostart" type="checkbox" @change="queueSave" />
      </label>

      <div v-if="state.providers.length" class="providers">
        <p class="group-title">Sources</p>
        <label v-for="p in state.providers" :key="p.name" class="row inline">
          <span>{{ p.name }}</span>
          <input
            type="checkbox"
            :checked="providerEnabled(p.name)"
            @change="toggleProvider(p.name, $event.target.checked)"
          />
        </label>
      </div>
    </section>

    <footer>
      <span>HaloBattery {{ state.version }} · {{ state.platform }}</span>
      <button class="ghost" @click="Halo.Quit()">Quit</button>
    </footer>
  </main>

  <main v-else class="shell loading">
    <p>Loading…</p>
  </main>
</template>
