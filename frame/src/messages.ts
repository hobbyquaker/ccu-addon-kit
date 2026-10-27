// The frame's own strings (keys `kit.*`); an addon's strings never use that prefix. Added to
// occulite-ui's catalogue by mountFrame(), under the addon's, so the addon's keys win.

import type {Catalogue} from 'occulite-ui/i18n';

export const FRAME_MESSAGES: Catalogue = {
    // tabs
    'kit.tab.service': {de: 'Dienst', en: 'Service'},
    'kit.tab.log': {de: 'Protokoll', en: 'Log'},
    'kit.tab.about': {de: 'Info', en: 'About'},

    // the status pill
    'kit.status.running': {de: 'Läuft', en: 'Running'},
    'kit.status.stopped': {de: 'Gestoppt', en: 'Stopped'},
    'kit.status.failed': {de: 'Fehler', en: 'Failed'},
    'kit.status.unknown': {de: 'Unbekannt', en: 'Unknown'},
    'kit.status.starting': {de: 'Startet …', en: 'Starting …'},
    'kit.status.stopping': {de: 'Stoppt …', en: 'Stopping …'},
    'kit.status.restarting': {de: 'Startet neu …', en: 'Restarting …'},
    'kit.status.reloading': {de: 'Lädt neu …', en: 'Reloading …'},
    'kit.status.wait': {de: 'Bitte warten …', en: 'Please wait …'},
    'kit.status.ram': {de: 'RAM {ram}', en: 'RAM {ram}'},
    'kit.status.ramOf': {de: '{ram} von {total}', en: '{ram} of {total}'},
    'kit.status.since': {de: 'seit {duration}', en: 'for {duration}'},
    'kit.status.unreadable': {de: 'Der Status konnte nicht gelesen werden.', en: 'The status could not be read.'},
    'kit.status.raw': {de: 'Ausgabe des Statusbefehls', en: 'Output of the status command'},

    // service actions
    'kit.cmd.start': {de: 'Start', en: 'Start'},
    'kit.cmd.stop': {de: 'Stopp', en: 'Stop'},
    'kit.cmd.restart': {de: 'Neustart', en: 'Restart'},
    'kit.cmd.reload': {de: 'Neu laden', en: 'Reload'},
    'kit.ask.stop.title': {de: 'Dienst stoppen', en: 'Stop the service'},
    'kit.ask.stop.message': {de: '{name} wirklich stoppen?', en: 'Really stop {name}?'},
    'kit.ask.restart.title': {de: 'Dienst neu starten', en: 'Restart the service'},
    'kit.ask.restart.message': {de: '{name} wirklich neu starten?', en: 'Really restart {name}?'},
    'kit.cmd.done': {de: '{cmd}: erledigt', en: '{cmd}: done'},
    'kit.cmd.failed': {de: '{cmd} fehlgeschlagen', en: '{cmd} failed'},
    'kit.cmd.output': {de: 'Ausgabe der letzten Aktion', en: 'Output of the last action'},
    'kit.open': {de: 'Öffnen', en: 'Open'},
    'kit.app.needsService': {de: 'Die Anwendung ist erreichbar, sobald der Dienst läuft.', en: 'The application is reachable once the service runs.'},

    // the Service tab
    'kit.service.state': {de: 'Zustand', en: 'State'},
    'kit.service.pid': {de: 'PID', en: 'PID'},
    'kit.service.ram': {de: 'Arbeitsspeicher', en: 'Memory'},
    'kit.service.share': {de: '{pct} % des Systems', en: '{pct} % of the system'},
    'kit.service.uptime': {de: 'Laufzeit', en: 'Uptime'},
    'kit.service.version': {de: 'Version', en: 'Version'},
    'kit.service.lastError': {de: 'Letzter Fehler', en: 'Last error'},
    'kit.service.toLog': {de: 'Zum Protokoll', en: 'To the log'},
    'kit.service.noRights': {de: 'Diese Sitzung darf den Dienst nicht steuern.', en: 'This session may not control the service.'},

    // the Log tab
    'kit.log.source': {de: 'Quelle', en: 'Source'},
    'kit.log.lines': {de: 'Zeilen', en: 'Lines'},
    'kit.log.filter': {de: 'Filtern', en: 'Filter'},
    'kit.log.auto': {de: 'Automatisch aktualisieren', en: 'Refresh automatically'},
    'kit.log.refresh': {de: 'Aktualisieren', en: 'Refresh'},
    'kit.log.download': {de: 'Herunterladen', en: 'Download'},
    'kit.log.bundle': {de: 'Diagnosepaket', en: 'Debug bundle'},
    'kit.log.empty': {de: 'Keine Einträge.', en: 'No entries.'},
    'kit.log.noMatch': {de: 'Keine Zeile passt zum Filter.', en: 'No line matches the filter.'},
    'kit.log.unavailable': {de: 'Diese Quelle ist nicht verfügbar.', en: 'This source is not available.'},
    'kit.log.journal': {
        de: 'Die Meldungen dieses Dienstes stehen im Protokoll des Systems.',
        en: "This service's messages are in the system's log.",
    },
    'kit.log.journalLink': {de: 'Protokoll des Systems öffnen', en: "Open the system's log"},
    'kit.log.truncated': {de: 'Ältere Zeilen sind in der Datei; „Herunterladen“ holt mehr.', en: 'Older lines are in the file; "Download" gets more.'},

    // the About tab
    'kit.about.installed': {de: 'Installierte Version', en: 'Installed version'},
    'kit.about.components': {de: 'Komponenten', en: 'Components'},
    'kit.about.kit': {de: 'ccu-addon-kit', en: 'ccu-addon-kit'},
    'kit.about.platform': {de: 'System', en: 'System'},
    'kit.about.latest': {de: 'Neueste Version', en: 'Latest version'},
    'kit.about.check': {de: 'Nach Updates suchen', en: 'Check for updates'},
    'kit.about.checking': {de: 'Suche …', en: 'Checking …'},
    'kit.about.newer': {de: 'Version {version} ist verfügbar.', en: 'Version {version} is available.'},
    'kit.about.current': {de: 'Die installierte Version ist aktuell.', en: 'The installed version is up to date.'},
    'kit.about.release': {de: 'Zur Release-Seite', en: 'To the release page'},
    'kit.about.offline': {de: 'Konnte nicht prüfen (keine Internetverbindung).', en: 'Could not check (no internet connection).'},
    'kit.about.rateLimited': {de: 'Konnte nicht prüfen (zu viele Anfragen bei GitHub, später erneut).', en: 'Could not check (too many requests to GitHub, try later).'},
    'kit.about.noRelease': {de: 'Es gibt noch keine veröffentlichte Version.', en: 'There is no published release yet.'},
    'kit.about.notConfigured': {de: 'Dieses Addon nennt keine Update-Quelle.', en: 'This addon names no update source.'},
    'kit.about.checkFailed': {de: 'Konnte nicht prüfen ({code}).', en: 'Could not check ({code}).'},
    'kit.about.checkedAt': {de: 'Geprüft {time}', en: 'Checked {time}'},
    'kit.about.links': {de: 'Links', en: 'Links'},
    'kit.about.user': {de: 'Angemeldet als', en: 'Signed in as'},

    // header
    'kit.header.theme': {de: 'Design', en: 'Theme'},
    'kit.header.lang': {de: 'Sprache', en: 'Language'},

    // session, errors, loading
    'kit.session.expired': {
        de: 'Die Sitzung ist abgelaufen. Bitte die Seite aus der WebUI neu öffnen.',
        en: 'The session has expired. Please open this page again from the WebUI.',
    },
    'kit.session.expiredLite': {de: 'Die Sitzung ist abgelaufen.', en: 'The session has expired.'},
    'kit.session.none': {
        de: 'Keine gültige Sitzung. Bitte die Seite aus der WebUI öffnen.',
        en: 'No valid session. Please open this page from the WebUI.',
    },
    'kit.session.home': {de: 'Zur WebUI', en: 'To the WebUI'},
    'kit.session.signin': {de: 'Neu anmelden', en: 'Sign in again'},
    'kit.retry': {de: 'Erneut versuchen', en: 'Retry'},
    'kit.loading': {de: 'Wird geladen …', en: 'Loading …'},
    'kit.boundary': {de: 'Dieser Bereich konnte nicht geladen werden.', en: 'This section could not be loaded.'},
    'kit.details': {de: 'Details', en: 'Details'},
    'kit.dirty.title': {de: 'Ungespeicherte Änderungen', en: 'Unsaved changes'},
    'kit.dirty.message': {de: 'Die Änderungen in diesem Bereich sind nicht gespeichert. Trotzdem wechseln?', en: 'The changes in this section are not saved. Leave anyway?'},
    'kit.dirty.leave': {de: 'Verwerfen und wechseln', en: 'Discard and leave'},
    'kit.cancel': {de: 'Abbrechen', en: 'Cancel'},
    'kit.ok': {de: 'OK', en: 'OK'},
    'kit.noConfig': {
        de: 'Die Konfiguration des Addons (frame/addon.json) fehlt. Das Paket ist unvollständig.',
        en: "The addon's configuration (frame/addon.json) is missing. The package is incomplete.",
    },

    // error codes of the CGIs (KitError.code)
    'kit.error.session-invalid': {de: 'Die Sitzung ist nicht (mehr) gültig.', en: 'The session is not valid (any more).'},
    'kit.error.forbidden': {de: 'Diese Sitzung hat dafür keine Berechtigung.', en: 'This session has no permission for that.'},
    'kit.error.csrf': {de: 'Die Anfrage wurde als fremd abgewiesen.', en: 'The request was refused as foreign.'},
    'kit.error.method': {de: 'Diese Anfrage ist so nicht erlaubt.', en: 'This request is not allowed.'},
    'kit.error.bad-request': {de: 'Ungültige Anfrage.', en: 'Invalid request.'},
    'kit.error.not-found': {de: 'Nicht gefunden.', en: 'Not found.'},
    'kit.error.busy': {de: 'Eine andere Aktion läuft gerade.', en: 'Another action is running.'},
    'kit.error.timeout': {de: 'Keine Antwort in der vorgesehenen Zeit.', en: 'No answer in time.'},
    'kit.error.failed': {de: 'Der Befehl ist fehlgeschlagen.', en: 'The command failed.'},
    'kit.error.internal': {de: 'Interner Fehler des Addons.', en: 'Internal error of the addon.'},
    'kit.error.network': {de: 'Das System ist nicht erreichbar.', en: 'The system cannot be reached.'},
    'kit.error.other': {de: 'Fehler ({code}).', en: 'Error ({code}).'},
};
