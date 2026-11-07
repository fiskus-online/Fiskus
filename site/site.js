// fiskus.online: odkazy na stiahnutie podľa systému.
;(function () {
  const cfg = window.FISKUS || {}
  const v = cfg.version || ''

  // ---- stiahnutie ----
  const files = {
    windows: `fiskus-${v}-windows-setup.exe`,
    macos: `fiskus-${v}-macos.dmg`,
    deb: `fiskus_${v}_amd64.deb`,
    rpm: `fiskus-${v}-1.x86_64.rpm`,
  }
  document.querySelectorAll('[data-file]').forEach((a) => {
    const name = files[a.dataset.file]
    a.href = (cfg.downloads || '') + name
    a.setAttribute('download', '')
    const label = document.querySelector(`[data-name="${a.dataset.file}"]`)
    if (label) label.textContent = name
  })
  document.querySelectorAll('[data-version]').forEach((el) => (el.textContent = v))

  const ua = navigator.userAgent
  const os = /Windows/.test(ua) ? 'windows' : /Mac OS X/.test(ua) && !/iPhone|iPad/.test(ua) ? 'macos' : /Linux/.test(ua) && !/Android/.test(ua) ? 'linux' : ''
  if (os) {
    document.querySelector(`tr[data-os="${os}"]`)?.classList.add('mine')
    const hero = document.querySelector('[data-hero-download]')
    if (hero) {
      const target = os === 'linux' ? 'deb' : os
      hero.href = (cfg.downloads || '') + files[target]
      hero.setAttribute('download', '')
      hero.textContent = { windows: 'Stiahnuť pre Windows', macos: 'Stiahnuť pre macOS', linux: 'Stiahnuť pre Linux' }[os]
    }
  }
})()
