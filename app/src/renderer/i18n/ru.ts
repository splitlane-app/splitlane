// Russian dictionary: every UI string lives here. Flat keys, sorted by area.
export const ru = {
  // lanes and actions on cards
  'lane.vpn': 'Через VPN',
  'lane.direct': 'Напрямую',
  'lane.main': 'основная',
  'lane.makeMain': 'сделать основной',
  'move.vpn': 'Пустить через VPN',
  'move.direct': 'Пустить напрямую',
  unpin: 'Открепить',
  pin: 'Закрепить',
  'card.quiet': 'тихо',

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
  'notice.rejected': '{name}: {reason}'
} as const

export type Key = keyof typeof ru
