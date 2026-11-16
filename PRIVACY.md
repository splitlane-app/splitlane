# Privacy policy · Политика конфиденциальности

[English](#english) · [Русский](#русский)

## English

This policy covers SplitLane 0.3.0 and later versions until it is changed. Changes are made in this file in the repository.

### In short

SplitLane has no telemetry, no accounts of its own and no servers of its own. It doesn't collect your data and has nowhere to send it. The only requests that leave your PC are the ones described below, and they go to GitHub, Tailscale, the services named here, the VPN servers and subscription addresses you add, and the VPS you set up with **Your own VPS**.

### What leaves your PC and why

**1. The update check.** The window asks GitHub whether a newer version is out:

- `GET https://api.github.com/repos/splitlane-app/splitlane/releases/latest`, without logging in;
- when you click **Update**: the `.sha256` file and the installer of that release from `github.com`, which redirects to `objects.githubusercontent.com` or `release-assets.githubusercontent.com`. The window doesn't connect to any other host for updates.

Each request carries the header `User-Agent: SplitLane/<version>` and nothing about you. GitHub sees the IP address the request comes from, the version of SplitLane and the time ([GitHub's privacy statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement) applies).

When: 30 seconds after the window starts and then every 24 hours while it runs, if **Check for updates automatically** is on (on by default; the setting is on the **Connection** screen), and whenever you click **Check for updates**. Manual checks work with the setting off. **What's new** opens the release page in your browser.

**2. The embedded Tailscale node.** When you connect through Tailscale, the VPN lane goes through a Tailscale node inside SplitLane's network core. It is a separate device in your tailnet, named `<PC name>-splitlane`, and it talks to Tailscale's coordination and relay servers and to your exit node like any Tailscale device. You log it in on Tailscale's website in your browser. It runs while split routing is on, and while the window is running after you have logged in (to show the list of exit nodes). [Tailscale's privacy policy](https://tailscale.com/privacy-policy) applies. While you connect with a key, a subscription or a WireGuard file, the Tailscale node doesn't run.

**3. Connection checks.** While split routing is on, the service asks `https://api.ipify.org` for the external address, through the VPN lane and directly, about every 10 seconds and while turning split routing on. This proves that the VPN lane works and goes through the VPN server (the exit node or the server you chose), catches traffic that bypasses the VPN, and gives the addresses shown on the lanes. api.ipify.org sees the address of the VPN server and your real address.

**4. DNS.** While the VPN lane works, website names of all programs are resolved through encrypted DNS (DNS over HTTPS) at `1.1.1.1` (Cloudflare), sent through the VPN lane; your ISP doesn't see these lookups. When the VPN server is unreachable or **All direct** is on, names go to the DNS of your network connection, usually your ISP's. The core's own lookups, such as the addresses of Tailscale servers, always use the DNS of your network connection.

**5. Traffic of your programs** follows the lanes you chose: programs on the VPN lane go through your VPN (the embedded node and your exit node, or the server of your key, subscription or WireGuard file, whose operator sees this traffic as with any VPN), the rest goes through your normal connection. SplitLane doesn't read or record what programs send. To show live activity, the service reads from the core which program connects to which host on which lane, and how fast; this stays on your PC.

If the Tailscale app is installed, the service also runs its `tailscale` command on your PC to read its exit node and to turn it off and back on.

**6. Subscriptions.** When you add a subscription address or click **Refresh**, the service downloads the subscription from the address you gave: a `GET` request over https only, with the header `User-Agent: SplitLane/<version>`, at most 1 MB, 20 seconds and 5 redirects. Addresses in local networks (loopback, private and link-local addresses, Tailscale's `100.64.0.0/10`) are refused, after each redirect too, without connecting to them. While split routing is on, the download goes through the VPN lane. The server of the subscription sees the address the request comes from, the version of SplitLane, the time and the subscription address itself, which often holds a token. SplitLane doesn't refresh subscriptions on its own.

**7. Your own VPS.** When you click **Set up**, SplitLane's setup process (`splitlane-helper.exe server-setup`, which runs as you, without administrator rights) connects over SSH to the server you entered, signs in with the username and the password or key you gave, and runs commands there: it reads `/etc/os-release` and the architecture, checks the rights (`sudo`), uploads SplitLane's server kit (a shell script) to a temporary file, runs it and deletes it. On the server, the kit downloads sing-box from GitHub and packages from the mirrors of its distribution, asks `api.ipify.org` for the server's public address, and Caddy gets a certificate from Let's Encrypt. Then the setup process checks the new subscription address over HTTPS (for up to 90 seconds), and the service adds it like any subscription (see 6). The address is an `sslip.io` name, so DNS lookups of it go to the name servers of sslip.io.

The password, the key and its passphrase are used for this setup only. The window reads the key file (at most 16 KB) and keeps its path to itself; the credentials reach the setup process through its standard input, not its command line, are never written to disk, logs or diagnostics archives and never reach the service. The window forgets them when you leave its screen. SplitLane doesn't verify the server's SSH host key at the first connection (a freshly rented server has nothing to compare it with): someone who intercepts the connection at that very moment could learn the password. Signing in with a key avoids this.

### What is stored on your PC

- `%ProgramData%\SplitLane` (only administrators and SYSTEM can open it): the state of the embedded node, including its key; the settings of the service (your pinned programs and their paths, the main lane, the way you connect, the selected exit node or server, the list of exit nodes and the device name); `servers.json`, the servers and subscriptions you added (the links and WireGuard files with their keys and passwords, the subscription addresses); the core configuration (`core-config.json`, which also holds the keys and password of the selected server: it is written again each time split routing turns on, so the keys of a server you removed may stay in it until split routing is next turned on) and rule files; the record used to turn the exit node of the Tailscale app back on; the logs in `logs\`. Its `export\` folder, which you can read, holds diagnostics archives (the last five) and the result of the last installation.
- `%AppData%\SplitLane`: the window settings, the icon cache of programs and Electron's working files.
- `%TEMP%\SplitLane-update`: the installer downloaded for an update. It is deleted the next time the window starts.
- The value `SplitLane` in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, while **Start when I sign in to Windows** is on, and the Task Scheduler task `SplitLane Helper`.

The SSH access to a server you set up with **Your own VPS** isn't stored: neither the username nor the password or the key. The servers and the subscription the setup adds (with the server's address in them) are kept in `servers.json` like any others.

### Logs and diagnostics

Logs stay on your PC. The service is designed to keep IP addresses, Tailscale names, keys, passwords and subscription addresses out of its own log (it masks them); the log of the core records the hosts that programs connect to.

**Save log** (on the banner shown when split routing can't turn on) creates a diagnostics archive: the logs of the service and the core, the settings of the service and the versions. Login links of the node are hidden, and keys, passwords and subscription addresses in the core log are masked; the node key, the core configuration, `servers.json` and other secrets are never included. The archive still contains the paths of your programs (they can include your Windows user name), host names from the core log, the names and Tailscale addresses of exit nodes and the device name. Nothing is sent anywhere: you decide whether to share the archive. Look through it before you attach it to a public issue.

### How to delete everything

1. Uninstall SplitLane with the checkbox **Remove the settings and the Tailscale login** on. This deletes `%ProgramData%\SplitLane` and your `%AppData%\SplitLane`, along with the program, the task and the startup value. Silent: `"%ProgramFiles%\SplitLane\Uninstall SplitLane.exe" /S /PURGE`.
2. Remove the device `<PC name>-splitlane` in the [Tailscale admin console](https://login.tailscale.com/admin/machines). Uninstalling doesn't remove it from your tailnet.
3. Delete any diagnostics archives you saved elsewhere, for example in Downloads.

### Contact

Questions about this policy: [GitHub issues](https://github.com/splitlane-app/splitlane/issues). Security problems: see [SECURITY.md](SECURITY.md).

## Русский

Эта политика относится к SplitLane 0.3.0 и последующим версиям, пока её не изменят. Изменения вносятся в этот файл в репозитории.

### Коротко

У SplitLane нет телеметрии, собственных учётных записей и собственных серверов. Он не собирает ваши данные, и отправлять их ему некуда. С компьютера уходят только запросы, описанные ниже, — к GitHub, Tailscale, названным здесь сервисам, к VPN-серверам и адресам подписок, которые вы добавили, и к VPS, который вы настраиваете в «Свой VPS».

### Что уходит с компьютера и зачем

**1. Проверка обновлений.** Окно спрашивает GitHub, есть ли новая версия:

- `GET https://api.github.com/repos/splitlane-app/splitlane/releases/latest`, без входа;
- по кнопке «Обновить» — файл `.sha256` и установщик этого выпуска с `github.com`, который переадресует на `objects.githubusercontent.com` или `release-assets.githubusercontent.com`. К другим узлам за обновлениями окно не обращается.

В каждом запросе — заголовок `User-Agent: SplitLane/<версия>` и ничего о вас. GitHub видит IP-адрес, с которого пришёл запрос, версию SplitLane и время (действует [заявление GitHub о конфиденциальности](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement)).

Когда: через 30 секунд после запуска окна и затем раз в 24 часа, пока окно работает, если включена настройка «Проверять обновления» (по умолчанию включена; она на экране «Подключение»), а также каждый раз по кнопке «Проверить обновления». Ручная проверка работает и с выключенной настройкой. «Что нового» открывает страницу выпуска в браузере.

**2. Встроенный узел Tailscale.** При подключении через Tailscale дорожка VPN идёт через узел Tailscale внутри сетевого ядра SplitLane. Это отдельное устройство в вашей сети Tailscale с именем `<имя ПК>-splitlane`; как любое устройство Tailscale, оно обращается к серверам управления и ретрансляторам Tailscale и к вашему exit node. Вход в него выполняется на сайте Tailscale в браузере. Узел работает, пока деление включено, а также пока окно работает после входа (чтобы показать список exit node). Действует [политика конфиденциальности Tailscale](https://tailscale.com/privacy-policy). Пока вы подключаетесь ключом, подпиской или файлом WireGuard, узел Tailscale не работает.

**3. Проверки связи.** Пока деление включено, служба запрашивает внешний адрес у `https://api.ipify.org` — через дорожку VPN и напрямую, примерно раз в 10 секунд и при включении деления. Так она убеждается, что дорожка VPN работает и идёт через VPN-сервер (exit node или выбранный вами сервер), замечает трафик в обход VPN и получает адреса, которые показаны на дорожках. api.ipify.org видит адрес VPN-сервера и ваш настоящий адрес.

**4. DNS.** Пока дорожка VPN работает, имена сайтов для всех программ разрешаются через шифрованный DNS (DNS-over-HTTPS) `1.1.1.1` (Cloudflare), запросы идут по дорожке VPN; провайдер их не видит. Когда VPN-сервер недоступен или включён режим «Всё напрямую», имена уходят в DNS вашего подключения, обычно провайдера. Собственные запросы ядра, например адреса серверов Tailscale, всегда идут в DNS вашего подключения.

**5. Трафик программ** идёт по дорожкам, которые вы выбрали: программы дорожки VPN — через ваш VPN (встроенный узел и ваш exit node или сервер вашего ключа, подписки или файла WireGuard; его владелец видит этот трафик, как при любом VPN), остальные — через обычное подключение. SplitLane не читает и не записывает то, что передают программы. Для живой активности служба получает от ядра, какая программа к какому узлу подключена, по какой дорожке и с какой скоростью; эти данные остаются на компьютере.

Кроме того, если приложение Tailscale установлено, служба вызывает на компьютере его команду `tailscale`, чтобы узнать его exit node, выключить его и включить обратно.

**6. Подписки.** Когда вы добавляете адрес подписки или нажимаете «Обновить», служба скачивает подписку с указанного вами адреса: запрос `GET` только по https, с заголовком `User-Agent: SplitLane/<версия>`, не больше 1 МБ, 20 секунд и 5 переадресаций. Адреса местных сетей (loopback, частные и link-local адреса, сеть Tailscale `100.64.0.0/10`) запрещены — и после каждой переадресации, — к ним служба не подключается. Пока деление включено, загрузка идёт через дорожку VPN. Сервер подписки видит адрес, с которого пришёл запрос, версию SplitLane, время и сам адрес подписки, в котором часто есть токен. Сам по себе SplitLane подписки не обновляет.

**7. Свой VPS.** По кнопке «Настроить» программа настройки SplitLane (`splitlane-helper.exe server-setup`; она работает от вашего имени, без прав администратора) подключается по SSH к серверу, который вы указали, входит с вашими логином и паролем или ключом и выполняет там команды: читает `/etc/os-release` и архитектуру, проверяет права (`sudo`), загружает набор SplitLane для сервера (скрипт оболочки) во временный файл, запускает его и удаляет. На сервере набор скачивает sing-box с GitHub и пакеты с зеркал своего дистрибутива, узнаёт публичный адрес сервера у `api.ipify.org`, а Caddy получает сертификат у Let's Encrypt. Затем программа настройки проверяет новый адрес подписки по HTTPS (до 90 секунд), и служба добавляет его, как любую подписку (см. п. 6). Этот адрес — имя `sslip.io`, поэтому DNS-запросы о нём уходят к серверам имён sslip.io.

Пароль, ключ и пароль ключа нужны только для этой настройки. Файл ключа читает окно (не больше 16 КБ), путь к нему никуда не уходит; учётные данные попадают в программу настройки через её стандартный ввод, а не через командную строку, никогда не записываются на диск, в журналы и архивы диагностики и не попадают в службу. Окно забывает их, когда вы уходите с его экрана. Ключ SSH сервера при первом подключении SplitLane не проверяет (у только что арендованного сервера его не с чем сверить): тот, кто перехватит соединение именно в этот момент, может узнать пароль. Вход по ключу этого риска лишён.

### Что хранится на компьютере

- `%ProgramData%\SplitLane` (открыт только администраторам и SYSTEM): состояние встроенного узла, в том числе его ключ; настройки службы (закреплённые программы и их пути, основная дорожка, способ подключения, выбранный exit node или сервер, список exit node и имя устройства); `servers.json` — добавленные вами серверы и подписки (ссылки и файлы WireGuard с их ключами и паролями, адреса подписок); конфиг ядра (`core-config.json`, в нём есть и ключи и пароль выбранного сервера: файл записывается заново при каждом включении деления, поэтому ключи удалённого сервера могут оставаться в нём до следующего включения) и файлы правил; запись, по которой exit node приложения Tailscale включается обратно; журналы в `logs\`. В подкаталоге `export\`, который вам доступен для чтения, — архивы диагностики (последние пять) и итог последней установки.
- `%AppData%\SplitLane`: настройки окна, кэш значков программ и рабочие файлы Electron.
- `%TEMP%\SplitLane-update`: установщик, скачанный для обновления. Он удаляется при следующем запуске окна.
- Значение `SplitLane` в `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, пока включено «Запускать при входе в Windows», и задача планировщика `SplitLane Helper`.

Доступ по SSH к серверу, который вы настроили в «Свой VPS», не хранится: ни логин, ни пароль, ни ключ. Серверы и подписка, которые добавила настройка (в них есть адрес сервера), лежат в `servers.json`, как любые другие.

### Журналы и диагностика

Журналы остаются на компьютере. Служба устроена так, чтобы в её собственный журнал не попадали IP-адреса, имена Tailscale, ключи, пароли и адреса подписок (она их скрывает); журнал ядра записывает узлы, к которым подключаются программы.

«Сохранить журнал» (на плашке, которая появляется, если деление не удалось включить) создаёт архив диагностики: журналы службы и ядра, настройки службы и версии. Ссылки для входа узла скрыты, ключи, пароли и адреса подписок в журнале ядра замаскированы; ключ узла, конфиг ядра, `servers.json` и другие секреты в архив не попадают никогда. Но в архиве остаются пути ваших программ (в них может быть имя пользователя Windows), имена узлов из журнала ядра, имена и адреса exit node в сети Tailscale и имя устройства. Никуда архив не отправляется: делиться им или нет, решаете вы. Просмотрите его, прежде чем прикладывать к открытому issue.

### Как удалить всё

1. Удалите SplitLane с галочкой «Удалить настройки и вход в Tailscale». Вместе с программой, задачей и записью автозапуска удаляются `%ProgramData%\SplitLane` и ваш `%AppData%\SplitLane`. Тихо: `"%ProgramFiles%\SplitLane\Uninstall SplitLane.exe" /S /PURGE`.
2. Удалите устройство `<имя ПК>-splitlane` в [консоли администратора Tailscale](https://login.tailscale.com/admin/machines). Удаление программы не убирает его из сети Tailscale.
3. Удалите архивы диагностики, которые вы сохраняли в другие места, например в «Загрузки».

### Связь

Вопросы об этой политике: [GitHub issues](https://github.com/splitlane-app/splitlane/issues). Уязвимости: см. [SECURITY.md](SECURITY.md).
