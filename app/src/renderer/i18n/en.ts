import type { Key } from './ru'

// English dictionary: the keys and the {…} parameters of ru.ts, in its order. The terms follow the glossary of the
// plan of stage 2 part 3 (Via VPN, Direct, split routing, service, exit node…); a calm tone, no exclamation marks.
export const en: Record<Key, string> = {
  // the product and the separator of the parts of one line ("Tailscale · node · path")
  'app.name': 'SplitLane',
  sep: ' · ',

  // header (spec 9.1)
  'header.routing': 'Split routing',
  'header.on': 'On',
  'header.off': 'Off',
  'header.starting': 'Turning on…',
  'header.stopping': 'Turning off…',
  'header.search': 'Find a program',
  'header.add': '+ Add',
  'header.settings': 'Settings and connection',
  'pill.tailscale': 'Tailscale',
  'pill.direct': 'direct connection',
  'pill.relay': 'via a relay',
  'pill.noExitNode': 'no exit node selected',

  // lanes and actions on cards
  'lane.vpn': 'Via VPN',
  'lane.direct': 'Direct',
  'lane.main': 'main',
  'lane.makeMain': 'make main',
  'lane.vpnSees': 'Websites see the address of the {name} server',
  'lane.directSees': 'Websites see your real address',
  'lane.showAddress': 'Show the full address',
  'lane.others': 'other programs',
  'lane.othersOff': 'Unpinned programs appear here when split routing is on',
  'lane.dropHint': 'Drag a program or a shortcut here',
  'move.vpn': 'Send via VPN',
  'move.direct': 'Send direct',
  unpin: 'Unpin',
  pin: 'Pin',
  'card.quiet': 'quiet',
  // a program the helper routes by a rule of its own (the system Tailscale, SplitLane itself): it never moves
  'card.builtin': 'built-in rule',
  // what a screen reader says of a card: its name and lane, its kind; the menu of its actions
  'card.label': '{name}, {lane}',
  'card.labelUnpinned': '{name}, {lane}, not pinned',
  'card.labelBuiltin': '{name}, {lane}, built-in rule',
  'card.role': 'program card',
  'menu.label': 'Actions for {name}',

  // the add panel (spec 9.3)
  'add.title': 'Add a program',
  'add.close': 'Close',
  'add.tabs': 'Where to find the program',
  'add.tabOnline': 'Online now',
  'add.tabInstalled': 'Installed',
  'add.pick': 'Browse…',
  'add.picked': 'Selected file',
  'add.onlineOff': 'Turn on split routing to see the programs that are online now',
  'add.onlineEmpty': 'No unpinned programs are online now',
  'add.installedLoading': 'Looking for installed programs…',
  'add.installedEmpty': 'No installed programs found',
  'add.installedFailed': "Couldn't get the list of installed programs",
  'add.dragHint': 'You can drag a row onto a lane',
  'add.here': 'now: {lane}',
  'add.pinnedIn': 'pinned: {lane}',
  'add.builtinIn': 'built-in rule: {lane}',
  // the header's search hides every row of the list
  'add.searchEmpty': 'Nothing found',

  // the program card (spec 9.4)
  'program.close': 'Close',
  'program.statePinnedVpn': 'pinned · via VPN',
  'program.statePinnedDirect': 'pinned · direct',
  'program.stateUnpinned': 'not pinned · follows the main lane',
  'program.stateBuiltinVpn': 'built-in rule · via VPN',
  'program.stateBuiltinDirect': 'built-in rule · direct',
  'program.identity': 'What counts as this program',
  'program.wholeFolder': 'The whole program folder',
  'program.wholeFolderOn': 'Every file in this folder counts as this program',
  'program.wholeFolderOff': 'Only this file counts',
  'program.wholeFolderFixed': "Microsoft Store apps have their own rule, so this setting isn't available",
  'program.path': 'Path',
  'program.exes': 'Detected files',
  'program.noExes': 'None detected yet',
  'program.connections': 'Current connections',
  'program.noConnections': 'No connections',
  'program.connectionsOff': 'Connections are shown while split routing is on',
  'program.colHost': 'Host',
  'program.colLane': 'Lane',
  'program.colSpeed': 'Speed',

  // dragging a card (said by the screen reader)
  'dnd.instructions':
    'Press Space to pick up a card, the arrow keys to move it, Space to drop it, Escape to cancel. Press Enter to open the program card, Shift+F10 for actions',
  'dnd.picked': '{name}: picked up',
  'dnd.action': '{name}: {action}',
  'dnd.stay': '{name}: stays in place',
  'dnd.canceled': '{name}: move canceled',
  'dnd.addAction': '{name}: pin, {lane}',

  // speeds: down and up of a lane
  'speed.down': '↓ {speed}',
  'speed.up': '↑ {speed}',

  // special tiles of the board (spec 8.4)
  'tile.system': 'Windows services',
  'tile.unattributed': 'WSL and virtual machines',

  // speed units and the decimal separator of the numbers
  'unit.bps': 'B/s',
  'unit.kbps': 'KB/s',
  'unit.mbps': 'MB/s',
  'unit.decimal': '.',

  // state banners (spec 9.5)
  'banner.off': 'Off. Programs connect as usual, without split routing',
  'banner.starting': 'Turning on…',
  'banner.degraded':
    "The {name} server is unreachable. Programs in the VPN lane have no connection for now, and website names are temporarily resolved by your ISP's DNS",
  'banner.serverFallback': 'VPN',
  'banner.loginAgain': 'You need to log in to Tailscale again. Split routing will be off while you log in',
  'banner.blocked': 'The Tailscale app is using an exit node. This conflicts with split routing',
  'banner.stopping': 'Turning off…',
  'banner.restorePending': "Can't turn the exit node back on in the Tailscale app",
  'banner.restoreHint': 'Run this in Command Prompt:',
  'banner.error': "Couldn't turn on split routing: {reason}",
  'banner.errorUnknown': "Couldn't turn on split routing",
  'banner.allDirect': '“All direct” is on: every program bypasses the VPN',
  'banner.restarting': 'The service is restarting…',
  'banner.unresponsive': 'The service is not responding',

  // banner actions
  'action.enable': 'Turn on',
  'action.cancel': 'Cancel',
  'action.retry': 'Retry',
  'action.allDirectOn': 'All direct',
  'action.allDirectOff': 'Back to normal mode',
  'action.loginAgain': 'Turn off split routing and log in',
  'action.clearSystemExitNode': 'Turn it off',
  'action.disable': 'Turn off split routing',
  'action.skipRestore': 'Turn off without restoring',
  'action.saveLog': 'Save log',
  'action.retryLink': 'Retry',

  // updates of the window (stage 2 part 2)
  'update.available': 'Version {v} is available',
  'update.downloading': 'Downloading version {v}: {p}%',
  'update.verifying': 'Verifying version {v}…',
  'update.launching': 'Installing version {v}: Windows will ask you to confirm',
  'update.cancelled': 'Update canceled',
  'update.downloadFailed': "Couldn't download version {v}: {reason}",
  'update.installed': 'SplitLane was updated to version {v}',
  'update.installRefused': "Version {v} wasn't installed: {reason}",
  'update.installRefusedNoReason': "Version {v} wasn't installed",
  'update.checking': 'Checking for updates…',
  'update.upToDate': "You're up to date",
  'update.checkFailed': "Couldn't check for updates: {reason}",
  'update.action.install': 'Update',
  'update.action.notes': "What's new",
  'update.action.later': 'Later',
  'update.action.cancel': 'Cancel',
  'update.action.retry': 'Retry',
  'update.action.dismiss': 'Close',
  'update.check': 'Check for updates',

  // the reasons of the updater: the main process gives them in English, model/update.ts maps the known ones here
  'update.reason.notInstalled': "this copy of SplitLane isn't installed",
  'update.reason.notStarted': "the installer didn't start the update",
  'update.reason.cancelled': 'canceled',
  'update.reason.aborted': 'the request was interrupted',
  'update.reason.releaseGone': "the release isn't available",
  'update.reason.checksumTooLarge': 'the checksum file is larger than 1 KB',
  'update.reason.checksumInvalid': "the checksum file isn't valid",
  'update.reason.fileLargerThanRelease': 'the file is larger than the release says',
  'update.reason.fileTooLarge': 'the file is larger than 400 MB',
  'update.reason.checksumMismatch': "the file doesn't match its checksum",
  'update.reason.answerTooLarge': 'the release information is larger than 1 MB',
  'update.reason.answerNotJson': "the release information isn't valid JSON",
  'update.reason.stalled': 'no data received for 60 seconds',
  'update.reason.addressNotAllowed': "the address isn't allowed",
  'update.reason.redirectNotAllowed': "redirected to a host that isn't allowed",
  'update.reason.tooManyRedirects': 'too many redirects',
  'update.reason.redirectNoLocation': 'redirected without an address',
  'update.reason.redirectBadAddress': 'redirected to an invalid address',
  'update.reason.http403': 'HTTP 403 (GitHub request limit reached?)',
  'update.reason.http': 'HTTP error {status}',
  'update.reason.noBody': 'the server sent no data',
  'update.reason.powershellExit': 'PowerShell exited with code {code}',
  'update.reason.releaseNotObject': "the release information isn't valid",
  'update.reason.releaseDraft': 'the release is a draft or a prerelease',
  'update.reason.releaseTag': "the release tag isn't valid",
  'update.reason.currentVersion': "the current version number isn't valid",
  'update.reason.releaseFiles': 'the release must have exactly one file named {name}',
  'update.reason.releaseAddress': 'the release file has an unexpected address',
  'update.reason.installerSize': 'the installer size is out of range',

  // reasons (state.reason.code): they follow "Couldn't turn on split routing: "
  'reason.no_default_route': 'this PC has no network connection',
  'reason.ip_forwarding':
    "Internet Connection Sharing or Mobile hotspot is on, and split routing doesn't work with them",
  'reason.other_core': 'another program that uses the sing-box network core is already running',
  'reason.not_logged_in': 'not logged in to Tailscale',
  'reason.no_exit_node': 'no exit node selected',
  'reason.system_dns_override':
    "Tailscale overrides DNS for all names (“Override local DNS” in its admin console), which split routing doesn't support",
  'reason.config_invalid': 'the network core settings failed validation (a SplitLane bug; save the log)',
  'reason.core_start_timeout': "the network core didn't start within 30 seconds",
  'reason.tunnel_not_proven': "the tunnel through the exit node couldn't be verified",
  'reason.same_address': "the exit node reaches the internet from the same address as this PC, so it can't be used",
  'reason.switch_check_failed': 'the check after switching failed',
  'reason.leak_detected': 'VPN lane traffic bypassed the VPN, so split routing was turned off for safety',
  'reason.core_crashed': "the network core crashed and didn't restart",
  'reason.core_unresponsive': 'the network core stopped responding',
  'reason.vpn_unreachable': 'the VPN server (exit node) is unreachable',
  'reason.login_expired': 'the Tailscale login has expired',
  'reason.restore_pending': "can't turn the exit node back on in the Tailscale app",
  'reason.adapter_left': 'the SplitLane virtual adapter is still there after turning off; restart your PC',

  // command errors (CommandResult.error.code)
  'error.bad_request': "The request wasn't accepted (an app error)",
  'error.unknown_method': "The service doesn't recognize this command. Update SplitLane",
  'error.protocol_mismatch': "The versions of the window and the service don't match",
  'error.wrong_phase': "This action isn't available right now",
  'error.busy': 'The service is busy. Try again in a few seconds',
  'error.rule_rejected': 'The rule was rejected',
  'error.apply_failed': "Couldn't apply the layout, so the previous one was restored",
  'error.unknown_exit_node': "This exit node isn't in the list",
  'error.internal': 'No connection to the service, or an internal error',

  // rule rejection reasons (error.rules[].reason)
  'reject.too_broad': 'Too broad: a drive root, the Windows folder or all of Program Files',
  'reject.protected': "This is part of SplitLane or Tailscale and can't be moved",
  'reject.duplicate': 'This program is already pinned in the other lane',

  // notices over the board: one rejected rule, joined with "; " when there are several
  'notice.rejected': '{name}: {reason}',
  'notice.logSaved': 'Log saved',
  // files from Explorer that are neither programs nor shortcuts to them, joined with ", "
  'notice.notProgram': 'Not a program: {names}',
  // shortcuts (.url) of Steam or Epic games that are not found on this PC, joined with ", "; one and several
  'notice.gameLink': '{names}: this is a game shortcut. Start the game, and it will appear in “Online now”',
  'notice.gameLinks': '{names}: these are game shortcuts. Start the games, and they will appear in “Online now”',
  'notice.close': 'Close',

  // the screens of the link (spec 4.3, 5.1)
  'screen.splash': 'Connecting to the service…',
  'screen.otherUser': 'The service is set up for another Windows user',
  'screen.otherUserLead':
    'SplitLane on this PC was set up by another Windows user. Sign in to Windows as that user, or ask them to reinstall SplitLane',
  'screen.updateApp': 'Update the app',
  'screen.updateAppLead': 'The service is newer than this window. Update SplitLane so that they work together again',

  // an error while drawing the window (the error boundary of the page)
  'crash.title': 'Window error',
  'crash.lead': 'Something went wrong in the window. Split routing and the service are still running',
  'crash.reload': 'Reload the window',

  // setup and update of the helper (spec 4.3, 5.1)
  'setup.title': 'First-time setup',
  'setup.lead':
    'To split program traffic, SplitLane needs a background service with administrator rights. You install it once, and Windows asks you to confirm. After that, the window works without administrator rights.',
  'setup.install': 'Install the service',
  'setup.updateTitle': 'Update the service',
  'setup.updateLead': 'The service is older than this window. The update installs the same way, and Windows asks you to confirm',
  'setup.update': 'Update the service',
  'setup.running': 'Confirm the Windows prompt…',
  'setup.done': 'Done. The service is starting…',
  'setup.cancelled': 'Setup canceled',
  'setup.failed': "Couldn't complete the setup",
  'setup.notInstalled':
    "This copy of SplitLane isn't installed. Run the SplitLane installer, which also sets up the background service.",

  // connect the VPN and the connection screen (spec 9.5, 9.6)
  'connect.title': 'Connect a VPN',
  'connect.lead': 'Choose how SplitLane connects to a VPN',
  'connect.methods': 'Connection methods',
  'connect.tailscale': 'Tailscale',
  'connect.tailscaleNote': 'available',
  'connect.key': 'Key or subscription',
  'connect.file': 'Configuration file',
  'connect.soon': 'coming soon',
  'connect.steps': 'Steps',
  'connect.step.login': 'Log in to Tailscale',
  'connect.step.exitNode': 'Choose an exit node',
  'connect.step.enable': 'Turn on split routing',
  'connect.loginHint': 'The login page opens in your browser',
  'connect.loginButton': 'Log in to Tailscale',
  'connect.waitLogin':
    'The login page is open in your browser. Log in there, and this window will continue automatically',
  'connect.openAgain': 'Open the link again',
  'connect.openFailed': "Couldn't open the login link",
  'connect.approval':
    "The link has been used. This device is waiting for approval from your Tailscale network admin. This window will continue automatically once it's approved",
  'connect.exitNodeHint': 'Choose the server the VPN lane goes through',
  'connect.noExitNodes': 'Your Tailscale network has no exit nodes',
  'connect.offline': 'offline',
  'connect.offlineWarn': 'This node is offline, so the VPN lane may not work',
  'connect.exitNodes': 'Exit nodes',
  'connect.systemWarn':
    'The Tailscale app is using an exit node. SplitLane turns it off while it runs and turns it back on when you turn off split routing',
  'connect.enable': 'Turn on split routing',
  'connect.enabling': 'Turning on…',
  'connection.title': 'Connection',
  'connect.versions': 'SplitLane {app} · service {helper} · core {core}',
  'connect.versionApp': 'SplitLane {app}',
  'connection.device': 'Device on the Tailscale network',
  'connection.exitNode': 'Selected exit node',
  'connection.changeNote': 'Changing the exit node briefly restarts split routing',
  'connection.logout': 'Log out of Tailscale',
  'connection.logoutBlocked': 'You can log out when split routing is off',
  'connection.openAtLogin': 'Start when I sign in to Windows',
  'connection.enableOnStart': 'Turn on split routing at startup',
  'connection.checkUpdates': 'Check for updates automatically',
  'connection.settings': 'Settings',
  'connection.saveFailed': "Couldn't save the setting",
  'connection.language': 'Language',
  'connection.languageSystem': 'Same as Windows',
  'screen.back': 'Back to lanes',

  // the names of the languages: each in its own language, the same in both dictionaries
  'language.ru': 'Русский',
  'language.en': 'English'
}
