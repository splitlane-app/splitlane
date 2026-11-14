# Сторонние компоненты

## sing-box

- Версия: 1.14.4, официальная сборка `windows-amd64`, без изменений. Файл ядра сохраняет имя `sing-box.exe`.
- Лицензия: GNU GPL версии 3 или более поздней, с дополнительным условием автора: производная работа не может использовать название sing-box или намекать на связь с ним без согласия автора. Поэтому слово «sing-box» не входит в название SplitLane, задачи планировщика, канала и собственных исполняемых файлов. Текст лицензии ядра — файл `LICENSE` в архиве ядра; рядом со служебным процессом он лежит под именем `sing-box-LICENSE`.
- Исходный код этой версии: https://github.com/SagerNet/sing-box/tree/v1.14.4
- Архив: https://github.com/SagerNet/sing-box/releases/download/v1.14.4/sing-box-1.14.4-windows-amd64.zip
- Вместе с ядром из того же архива без изменений поставляется `libcronet.dll` (библиотека проекта cronet-go: https://github.com/sagernet/cronet-go); ядро загружает её из своей папки.
- SHA-256 архива, `sing-box.exe` и `libcronet.dll`: `scripts/core.sha256`. Ядро в git не хранится, его скачивает `scripts/fetch-core.ps1`.

## Модули Go служебного процесса

- `golang.org/x/sys` v0.49.0 — лицензия BSD-3-Clause, https://cs.opensource.google/go/x/sys
- `github.com/Microsoft/go-winio` v0.6.4 — лицензия MIT, https://github.com/microsoft/go-winio
