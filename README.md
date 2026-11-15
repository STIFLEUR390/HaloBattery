# HaloBattery (portage Go / Wails)

Portage en Go du projet Python [HaloBattery](https://github.com/Lucas-Fer/HaloBattery) :
niveaux de batterie des souris, claviers et casques sans fil, dans la barre système
et dans une petite fenêtre de tableau de bord.

Ce dépôt est une réécriture, pas un wrapper : le protocole HID des appareils Razer
et Logitech est réimplémenté en Go, sans hidapi ni cgo.

## État

| Fonction | État |
| --- | --- |
| Fenêtre tableau de bord (Wails v3 + Vue) | ✅ |
| Icône de barre système (anneau coloré) | ✅ |
| Razer (feature reports 90 octets, CRC XOR) | ✅ |
| Logitech HID++ 1.0 / 2.0 (récepteurs Unifying/Bolt/Lightspeed) | ✅ |
| UPower (D-Bus) — périphériques déjà connus du système | ✅ |
| Bluetooth via BlueZ (`org.bluez.Battery1`) — souris, claviers et casques BLE | ✅ |
| Alertes batterie faible / pleine (notifications) | ✅ |
| Intervalle, seuil, sources désactivables, démarrage auto | ✅ |
| Icône par appareil, menu volant façon Windows 11, historique, mise à jour auto, i18n, renommage | ❌ (voir plus bas) |

## Compiler

Dépendances : Go ≥ 1.24, Node ≥ 20, GTK ≥ 4.14 + webkitgtk-6.0, `gcc`.
Sous Fedora 45 :

```bash
sudo dnf install gcc pkgconf-pkg-config glib2-devel gtk4-devel \
  webkitgtk6.0-devel libsoup3-devel
```

Sous Debian/Ubuntu : `libgtk-4-dev`, `libwebkitgtk-6.0-dev`, `libsoup-3.0-dev`.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
npm --prefix frontend install
wails3 build                  # -> bin/halobattery
wails3 dev                    # rechargement à chaud
wails3 task linux:create:deb  # -> bin/halobattery.deb
```

Wails v3 utilise par défaut GTK4 + webkitgtk-6.0, le backend retenu ici.
Sur une distribution qui n'a que GTK3 (< 4.14), forcer la variante historique
avec `EXTRA_TAGS: 'gtk3'` dans `build/linux/Taskfile.yml`. Équivalent manuel :

```bash
go build -tags production -o bin/halobattery .
go test ./internal/...
```

## Accès aux appareils HID

Sous Linux, `/dev/hidraw*` appartient à root. Sans règle udev, l'application
détecte les appareils mais ne peut pas lire leur batterie :

```bash
sudo cp build/linux/halobattery.rules /etc/udev/rules.d/70-halobattery.rules
sudo udevadm control --reload-rules && sudo udevadm trigger
```

Puis débrancher/rebrancher le récepteur. `halobattery --probe` affiche le JSON des
appareils détectés (et les notes par source) sans lancer l'interface :

```json
{
  "devices": [
    { "key": "bluez:b9:18:08:9b:97:23", "name": "oraimo BoomPop Lite",
      "level": null, "source": "bluetooth", "kind": "headset", "online": true }
  ],
  "notes": ["Bluetooth, no battery service: oraimo BoomPop Lite"]
}
```

Un appareil Bluetooth connecté qui n'expose pas le service de batterie (Battery
Service BLE) est listé avec un niveau inconnu : BlueZ n'a alors rien à lire, et
aucun outil Linux ne peut inventer le pourcentage.

## Architecture

```
main.go                  fenêtre, icône de barre système, menu, --probe
service.go               état exposé au frontend, alertes, sauvegarde config
internal/hid             hidraw Linux pur Go (ioctl get/set feature), stubs ailleurs
internal/providers/razer    protocole Razer (feature report 0x00, 90 octets)
internal/providers/logitech HID++ 1.0 et 2.0, récepteurs, cache par slot
internal/providers/bluez    D-Bus BlueZ (Battery1 des appareils Bluetooth connectés)
internal/providers/upower   D-Bus UPower (niveau, état de charge, type)
internal/monitor         boucle de polling, fusion et dédoublonnage des sources
internal/icons           rendu PNG de l'anneau 64 px (4×4 supersampling)
assets/logo.png          icône de l'application (reprise de docs/logo.png du projet d'origine)
internal/config          config.json dans $XDG_CONFIG_HOME/HaloBattery
```

Chaque source implémente `providers.Provider` (`Name()`, `Poll() []device.Status`).
Ajouter un protocole = un paquet + une ligne dans `internal/monitor/providers_linux.go`.

## Différences assumées avec l'original

- **Une seule icône de barre système** avec la liste des appareils dans son menu,
  au lieu d'une icône par appareil : l'API tray de Wails v3 ne garantit pas
  plusieurs icônes, et les barres Wayland sont déjà encombrées.
- **Pas de reprise de l'installeur Inno Setup, du menu volant XAML, de l'historique,
  du vérificateur de mises à jour ni des 6 traductions.** Ce sont des couches Windows
  ou du confort ; le cœur (lecture des batteries, alertes, réglages) est là.
- Le protocole Razer couvre les chemins feature-report utilisés par les souris,
  claviers et casques courants ; les modèles à protocole dédié (BlackShark 2020/V3,
  certains casques PA) ne sont pas encore portés.
- UPower et BlueZ remplacent la découverte Bluetooth de l'original : tout ce que
  le système expose déjà remonte sans code spécifique. Là où Windows lisait le
  pourcentage dans le GATT, Linux le lit dans `org.bluez.Battery1` — quand
  l'appareil publie ce service.
- Icône : `docs/logo.png` du dépôt d'origine, redimensionnée avec `magick` en
  `build/appicon.png` (1024), puis déclinée en `.ico`, `.icns` et PNG 128/256 px.

## Licence

MIT, comme le projet d'origine.
