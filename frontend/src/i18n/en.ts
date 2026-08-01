// en 国际化
export default {
  app: { title: 'BoxPanel', running: 'Running', stopped: 'Stopped' },
  nav: { dashboard: 'Dashboard', servers: 'Servers', groups: 'Groups', routing: 'Routing', subscriptions: 'Subscriptions', logs: 'Logs', settings: 'Settings' },
  dashboard: {
    status: 'Status', start: 'Start', stop: 'Stop', restart: 'Restart',
    sysProxy: 'System Proxy', enable: 'Enable', disable: 'Disable',
    currentServer: 'Current Server', none: 'None selected',
    traffic: 'Live Traffic', up: 'Up', down: 'Down', totalUp: 'Total Up', totalDown: 'Total Down',
    connections: 'Connections', noServer: 'Please import and select a server first',
  },
  servers: {
    title: 'Servers', import: 'Import', testAll: 'Test All', test: 'Test',
    delete: 'Delete', select: 'Use', selected: 'Selected', empty: 'No servers yet',
    importPlaceholder: 'Paste vless:// / vmess:// / trojan:// / ss:// / hysteria2:// / tuic:// links, one per line\nor sing-box JSON / Clash YAML / base64 subscription',
    confirmDelete: 'Delete server {name}?',
  },
  subs: {
    title: 'Subscriptions', add: 'Add', name: 'Name', url: 'URL', ua: 'User-Agent',
    interval: 'Refresh interval (hours)', lastRefresh: 'Last refresh',
    refresh: 'Refresh now', refreshAll: 'Refresh all',
    confirmDelete: 'Delete subscription {name}?',
  },
  logs: { title: 'Live Logs', clear: 'Clear', autoScroll: 'Auto-scroll' },
  settings: {
    title: 'Settings', general: 'General', core: 'Core', network: 'Network',
    theme: 'Theme', dark: 'Dark', light: 'Light', language: 'Language',
    listenPort: 'Local listen port', clashApiPort: 'Clash API port',
    latencyUrl: 'Latency test URL', subUA: 'Subscription User-Agent',
    autoRefresh: 'Auto refresh subscriptions on start',
    save: 'Save', about: 'About', version: 'Version',
  },
}