// Russian dictionary: every UI string lives here. Flat keys, sorted by area.
export const ru = {
  // the product and the separator of the parts of one line ("Tailscale · node · path")
  'app.name': 'SplitLane',
  sep: ' · ',

  // header (spec 9.1)
  'header.routing': 'Деление программ по дорожкам',
  'header.on': 'Включено',
  'header.off': 'Выключено',
  'header.starting': 'Включается…',
  'header.stopping': 'Выключается…',
  'header.search': 'Найти программу',
  'header.add': '+ Добавить',
  'header.settings': 'Настройки и подключение',
  'pill.tailscale': 'Tailscale',
  'pill.direct': 'прямое соединение',
  'pill.relay': 'через ретранслятор',
  'pill.noExitNode': 'exit node не выбран',

  // lanes and actions on cards
  'lane.vpn': 'Через VPN',
  'lane.direct': 'Напрямую',
  'lane.main': 'основная',
  'lane.makeMain': 'сделать основной',
  'lane.vpnSees': 'Сайты видят адрес сервера {name}',
  'lane.directSees': 'Сайты видят ваш настоящий адрес',
  'lane.showAddress': 'Показать адрес полностью',
  'lane.others': 'остальные программы',
  'lane.othersOff': 'Незакреплённые программы появятся здесь после включения деления',
  'lane.dropHint': 'Перетащите сюда программу или ярлык',
  'move.vpn': 'Пустить через VPN',
  'move.direct': 'Пустить напрямую',
  unpin: 'Открепить',
  pin: 'Закрепить',
  'card.quiet': 'тихо',
  // a program the helper routes by a rule of its own (the system Tailscale, SplitLane itself): it never moves
  'card.builtin': 'встроенное правило',
  // what a screen reader says of a card: its name and lane, its kind; the menu of its actions
  'card.label': '{name}, {lane}',
  'card.labelUnpinned': '{name}, {lane}, не закреплена',
  'card.labelBuiltin': '{name}, {lane}, встроенное правило',
  'card.role': 'карточка программы',
  'menu.label': 'Действия: {name}',

  // the add panel (spec 9.3)
  'add.title': 'Добавить программу',
  'add.close': 'Закрыть',
  'add.tabs': 'Откуда взять программу',
  'add.tabOnline': 'Сейчас в сети',
  'add.tabInstalled': 'Установленные',
  'add.pick': 'Выбрать файл…',
  'add.picked': 'Выбранный файл',
  'add.onlineOff': 'Включите деление: тогда здесь появятся программы, которые сейчас выходят в сеть',
  'add.onlineEmpty': 'Сейчас в сети нет незакреплённых программ',
  'add.installedLoading': 'Ищем установленные программы…',
  'add.installedEmpty': 'Установленных программ не найдено',
  'add.installedFailed': 'Не удалось получить список установленных программ',
  'add.dragHint': 'Строку можно перетащить на дорожку',
  'add.here': 'сейчас: {lane}',
  'add.pinnedIn': 'закреплена: {lane}',
  'add.builtinIn': 'встроенное правило: {lane}',
  // the header's search hides every row of the list
  'add.searchEmpty': 'Ничего не найдено',

  // the program card (spec 9.4)
  'program.close': 'Закрыть',
  'program.statePinnedVpn': 'закреплена · через VPN',
  'program.statePinnedDirect': 'закреплена · напрямую',
  'program.stateUnpinned': 'не закреплена · по основной дорожке',
  'program.stateBuiltinVpn': 'встроенное правило · через VPN',
  'program.stateBuiltinDirect': 'встроенное правило · напрямую',
  'program.identity': 'Что считается этой программой',
  'program.wholeFolder': 'Вся папка программы',
  'program.wholeFolderOn': 'Все файлы из этой папки считаются этой программой',
  'program.wholeFolderOff': 'Считается только этот файл',
  'program.wholeFolderFixed': 'Для программы из магазина правило своё, переключатель недоступен',
  'program.path': 'Путь',
  'program.exes': 'Замеченные файлы',
  'program.noExes': 'Пока не замечены',
  'program.connections': 'Соединения сейчас',
  'program.noConnections': 'Нет соединений',
  'program.connectionsOff': 'Соединения видны, когда деление включено',
  'program.colHost': 'Узел',
  'program.colLane': 'Дорожка',
  'program.colSpeed': 'Скорость',

  // dragging a card (said by the screen reader)
  'dnd.instructions':
    'Пробел — взять карточку, стрелки — перенести, пробел — положить, Escape — отменить. Ввод — открыть карточку программы, Shift+F10 — действия',
  'dnd.picked': '{name}: перенос',
  'dnd.action': '{name}: {action}',
  'dnd.stay': '{name}: на своём месте',
  'dnd.canceled': '{name}: перенос отменён',
  'dnd.addAction': '{name}: закрепить, {lane}',

  // speeds: down and up of a lane
  'speed.down': '↓ {speed}',
  'speed.up': '↑ {speed}',

  // special tiles of the board (spec 8.4)
  'tile.system': 'Службы Windows',
  'tile.unattributed': 'WSL и виртуальные машины',

  // speed units
  'unit.bps': 'Б/с',
  'unit.kbps': 'КБ/с',
  'unit.mbps': 'МБ/с',

  // state banners (spec 9.5)
  'banner.off': 'Выключено. Программы ходят как обычно, без деления',
  'banner.starting': 'Включается…',
  'banner.degraded':
    'Сервер {name} недоступен. Программы дорожки VPN сейчас без связи. Имена сайтов временно разрешаются через DNS провайдера',
  'banner.serverFallback': 'VPN',
  'banner.loginAgain': 'Нужно снова войти в Tailscale. На время входа деление будет выключено',
  'banner.blocked': 'В приложении Tailscale включён exit node. Он мешает делению',
  'banner.stopping': 'Выключается…',
  'banner.restorePending': 'Не удаётся вернуть exit node в приложении Tailscale',
  'banner.restoreHint': 'Выполните в командной строке:',
  'banner.error': 'Не удалось включить: {reason}',
  'banner.errorUnknown': 'Не удалось включить',
  'banner.allDirect': 'Включён режим «всё напрямую»: все программы идут без VPN',
  'banner.restarting': 'Служебный процесс перезапускается…',
  'banner.unresponsive': 'Служебный процесс не отвечает',

  // banner actions
  'action.enable': 'Включить',
  'action.cancel': 'Отменить',
  'action.retry': 'Повторить',
  'action.allDirectOn': 'Всё напрямую',
  'action.allDirectOff': 'Вернуть обычный режим',
  'action.loginAgain': 'Выключить деление и войти',
  'action.clearSystemExitNode': 'Выключить его',
  'action.disable': 'Выключить деление',
  'action.skipRestore': 'Выключить без возврата',
  'action.saveLog': 'Сохранить журнал',
  'action.retryLink': 'Повторить',

  // reasons (state.reason.code)
  'reason.no_default_route': 'у компьютера нет подключения к сети',
  'reason.ip_forwarding': 'включён общий доступ к интернету или мобильная точка доступа, с ними деление не работает',
  'reason.other_core': 'уже запущена другая программа на сетевом ядре sing-box',
  'reason.not_logged_in': 'вход в Tailscale не выполнен',
  'reason.no_exit_node': 'exit node не выбран',
  'reason.system_dns_override':
    'Tailscale подменяет DNS для всех имён («Override DNS servers» в его консоли), с этим деление не включается',
  'reason.config_invalid': 'настройки ядра не прошли проверку (ошибка SplitLane, сохраните журнал)',
  'reason.core_start_timeout': 'сетевое ядро не запустилось за 30 секунд',
  'reason.tunnel_not_proven': 'не удалось убедиться, что туннель через exit node работает',
  'reason.same_address':
    'exit node выходит в интернет с того же адреса, что и этот компьютер, такой exit node не подходит',
  'reason.switch_check_failed': 'проверка после переключения не прошла',
  'reason.leak_detected': 'трафик дорожки VPN пошёл в обход VPN, деление выключено для безопасности',
  'reason.core_crashed': 'сетевое ядро аварийно завершилось и не перезапустилось',
  'reason.core_unresponsive': 'сетевое ядро перестало отвечать',
  'reason.vpn_unreachable': 'сервер VPN (exit node) недоступен',
  'reason.login_expired': 'срок входа в Tailscale истёк',
  'reason.restore_pending': 'не удаётся вернуть exit node в приложении Tailscale',
  'reason.adapter_left': 'после выключения остался виртуальный адаптер SplitLane, перезагрузите компьютер',

  // command errors (CommandResult.error.code)
  'error.bad_request': 'Запрос не принят (ошибка приложения)',
  'error.unknown_method': 'Служебный процесс не знает такой команды. Обновите приложение',
  'error.protocol_mismatch': 'Версии окна и служебного процесса не совпадают',
  'error.wrong_phase': 'Сейчас это действие недоступно',
  'error.busy': 'Служебный процесс занят. Повторите через несколько секунд',
  'error.rule_rejected': 'Правило отклонено',
  'error.apply_failed': 'Раскладку не удалось применить, прежняя возвращена',
  'error.unknown_exit_node': 'Такого exit node нет в списке',
  'error.internal': 'Нет связи со служебным процессом или внутренняя ошибка',

  // rule rejection reasons (error.rules[].reason)
  'reject.too_broad': 'Слишком много: корень диска, папка Windows или Program Files целиком',
  'reject.protected': 'Это часть SplitLane или Tailscale, её нельзя переносить',
  'reject.duplicate': 'Эта программа уже закреплена в другой дорожке',

  // notices over the board: one rejected rule, joined with "; " when there are several
  'notice.rejected': '{name}: {reason}',
  'notice.logSaved': 'Журнал сохранён',
  // files from Explorer that are neither programs nor shortcuts to them, joined with ", "
  'notice.notProgram': 'Не программа: {names}',
  // shortcuts (.url) of Steam or Epic games that are not found on this PC, joined with ", "; one and several
  'notice.gameLink': '{names}: это ярлык игры — запустите игру, и она появится в «Сейчас в сети»',
  'notice.gameLinks': '{names}: это ярлыки игр — запустите игры, и они появятся в «Сейчас в сети»',
  'notice.close': 'Закрыть',

  // the screens of the link (spec 4.3, 5.1)
  'screen.splash': 'Подключение к служебному процессу…',
  'screen.otherUser': 'Служебный процесс настроен для другого пользователя Windows',
  'screen.otherUserLead':
    'SplitLane на этом компьютере настроен другим пользователем Windows. Войдите под тем пользователем или попросите его всё переустановить',
  'screen.updateApp': 'Обновите приложение',
  'screen.updateAppLead': 'Служебный процесс новее этого окна. Обновите SplitLane, чтобы они снова работали вместе',

  // an error while drawing the window (the error boundary of the page)
  'crash.title': 'Сбой окна',
  'crash.lead': 'В окне произошла ошибка. Деление и служебный процесс работают как прежде',
  'crash.reload': 'Перезапустить окно',

  // setup and update of the helper (spec 4.3, 5.1)
  'setup.title': 'Первая настройка',
  'setup.lead':
    'Чтобы разделять трафик программ, SplitLane нужна фоновая служба с правами администратора. Она устанавливается один раз: Windows попросит подтвердить. Потом окно работает без прав администратора.',
  'setup.install': 'Установить службу',
  'setup.updateTitle': 'Обновите служебный процесс',
  'setup.updateLead': 'Служебный процесс старее этого окна. Обновление ставится так же, с подтверждением Windows',
  'setup.update': 'Обновить службу',
  'setup.running': 'Подтвердите запрос Windows…',
  'setup.done': 'Готово, служебный процесс запускается…',
  'setup.cancelled': 'Настройка отменена',
  'setup.failed': 'Не удалось выполнить настройку',
  'setup.notInstalled': 'Эта копия SplitLane не установлена. Установите SplitLane установщиком: он поставит и фоновую службу.',

  // connect the VPN and the connection screen (spec 9.5, 9.6)
  'connect.title': 'Подключите VPN',
  'connect.lead': 'Выберите, как SplitLane получит VPN',
  'connect.methods': 'Способы подключения',
  'connect.tailscale': 'Tailscale',
  'connect.tailscaleNote': 'доступно',
  'connect.key': 'Ключ или подписка',
  'connect.file': 'Файл настроек',
  'connect.soon': 'скоро',
  'connect.steps': 'Шаги',
  'connect.step.login': 'Войти в Tailscale',
  'connect.step.exitNode': 'Выбрать exit node',
  'connect.step.enable': 'Включить деление',
  'connect.loginHint': 'Откроется страница входа в браузере',
  'connect.loginButton': 'Войти в Tailscale',
  'connect.waitLogin': 'Страница входа открыта в браузере. Войдите там, и окно продолжит само',
  'connect.openAgain': 'Открыть ссылку снова',
  'connect.openFailed': 'Не удалось открыть ссылку входа',
  'connect.approval':
    'Ссылка использована, устройство ждёт одобрения администратором сети Tailscale. Окно продолжит само, когда его одобрят',
  'connect.exitNodeHint': 'Выберите сервер, через который пойдёт дорожка VPN',
  'connect.noExitNodes': 'В вашей сети Tailscale нет exit node',
  'connect.offline': 'не в сети',
  'connect.offlineWarn': 'Узел сейчас не в сети, дорожка VPN может не заработать',
  'connect.exitNodes': 'Exit node',
  'connect.systemWarn':
    'В приложении Tailscale сейчас включён exit node. Пока SplitLane работает, он будет выключен, а при выключении деления вернётся',
  'connect.enable': 'Включить деление',
  'connect.enabling': 'Включается…',
  'connection.title': 'Подключение',
  'connect.versions': 'SplitLane {app} · служба {helper} · ядро {core}',
  'connect.versionApp': 'SplitLane {app}',
  'connection.device': 'Устройство в сети Tailscale',
  'connection.exitNode': 'Выбранный exit node',
  'connection.changeNote': 'Смена exit node ненадолго перезапустит деление',
  'connection.logout': 'Выйти из Tailscale',
  'connection.logoutBlocked': 'Выйти можно, когда деление выключено',
  'connection.openAtLogin': 'Запускать при входе в Windows',
  'connection.enableOnStart': 'Включать деление при запуске',
  'connection.settings': 'Настройки',
  'connection.saveFailed': 'Не удалось сохранить настройку',
  'screen.back': 'Назад к дорожкам',
} as const

export type Key = keyof typeof ru
