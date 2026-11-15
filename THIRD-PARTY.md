# Сторонние компоненты

## sing-box

- Версия: 1.14.4, официальная сборка `windows-amd64`, без изменений. Файл ядра сохраняет имя `sing-box.exe`.
- Лицензия: GNU GPL версии 3 или более поздней, с дополнительным условием автора: производная работа не может использовать название sing-box или намекать на связь с ним без согласия автора. Поэтому слово «sing-box» не входит в название SplitLane, задачи планировщика, канала и собственных исполняемых файлов. Текст лицензии ядра — файл `LICENSE` в архиве ядра; рядом со служебным процессом он лежит под именем `sing-box-LICENSE`.
- Исходный код этой версии: https://github.com/SagerNet/sing-box/tree/v1.14.4
- Архив: https://github.com/SagerNet/sing-box/releases/download/v1.14.4/sing-box-1.14.4-windows-amd64.zip
- Вместе с ядром из того же архива без изменений поставляется `libcronet.dll` (раздел ниже); ядро загружает её из своей папки.
- SHA-256 архива, `sing-box.exe` и `libcronet.dll`: `scripts/core.sha256`. Ядро в git не хранится, его скачивает `scripts/fetch-core.ps1`.

## libcronet.dll

- Версия: файл `lib/windows_amd64/libcronet.dll` проекта cronet-go (https://github.com/SagerNet/cronet-go). sing-box 1.14.4 закрепляет cronet-go в `go.mod` (`github.com/sagernet/cronet-go v0.0.0-20260912104727-0d28acc44093`, модуль библиотеки `github.com/sagernet/cronet-go/lib/windows_amd64 v0.0.0-20260912104006-c10c03c318db`) и в `.github/CRONET_GO_VERSION` (`0d28acc44093df24b2526dea3d6ffefd6b0a54f0`); сборка выпуска (`.github/workflows/build.yml`, шаг «Extract libcronet.dll») кладёт в архив ядра этот файл из коммита `0d28acc4`. Поставляемый файл совпадает с ним побайтно (git blob `91268f1f6fbacf1f5a684428a3ff5b77a7f69527`).
- Исходный код: библиотека — сетевой стек Cronet из Chromium 150.0.7871.63. Её собрал коммит cronet-go `c10c03c318db6d47bec8705c8290bad34bc32b64` («Build from 049f2909») из коммита `049f2909701ca088c9569f6b303bba5376a2b2ff` с подмодулем naiveproxy: https://github.com/SagerNet/naiveproxy/tree/72a06c9fca0e2d228588c7f3074bf7efff3ff686 (каталог `src` — исходный код Chromium).
- Лицензия: исходный код Chromium — BSD-3-Clause, The Chromium Authors (файл `src/LICENSE` по ссылке выше). Она требует воспроизводить её текст при распространении в двоичном виде, поэтому текст приведён ниже: этот файл лежит рядом с `libcronet.dll` в `dist\helper` и в `%ProgramFiles%\SplitLane`. В библиотеку входят сторонние компоненты Chromium со своими лицензиями, в том числе BoringSSL (Apache-2.0 и BSD-3-Clause), Abseil (Apache-2.0), QUICHE (BSD-3-Clause), Protocol Buffers (лицензия в стиле BSD) и Perfetto (Apache-2.0); их перечень с лицензиями — файлы `README.chromium` и тексты лицензий в каталогах `src/third_party` и `src/net/third_party` того же коммита naiveproxy.
- Сам репозиторий cronet-go (привязки Go, они входят в `sing-box.exe`, и средства сборки) — GNU GPL версии 3 или более поздней, файл `LICENSE` в его корне.

Текст лицензии Chromium (`src/LICENSE` коммита naiveproxy `72a06c9f`):

```text
// Copyright 2015 The Chromium Authors
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google LLC nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## Установщик

- NSIS (Nullsoft Scriptable Install System) 3.04 в сборке electron-builder (`nsis-3.0.4.1`, модули `nsis-resources-3.4.1`): из него собраны установщик `SplitLane-Setup-<версия>.exe` и деинсталлятор `Uninstall SplitLane.exe`, его заглушка входит в оба файла. Лицензия zlib/libpng; отдельные части — свои лицензии, перечень: https://nsis.sourceforge.io/License
- Сценарии NSIS electron-builder (`app-builder-lib` 26.18.0, каталог `templates/nsis`), по которым собран установщик вместе с `app/build/installer.nsh`: лицензия MIT, https://github.com/electron-userland/electron-builder

## Модули Go служебного процесса

- `golang.org/x/sys` v0.49.0 — лицензия BSD-3-Clause, https://cs.opensource.google/go/x/sys
- `github.com/Microsoft/go-winio` v0.6.4 — лицензия MIT, https://github.com/microsoft/go-winio
