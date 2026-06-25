# rounting_list_util

## Русский

Небольшая Go-утилита для подготовки списков маршрутизации под раздельное
туннелирование в Amnezia VPN. Она скачивает `geoip.dat` или `geosite.dat` из
`runetfreedom/russia-v2ray-rules-dat`, извлекает выбранные категории и
записывает CIDR-диапазоны или домены в JSON, который можно использовать в
настройках маршрутизации Amnezia.

Утилита запускается из командной строки: откройте терминал в папке с
исполняемым файлом и выполните `./routing_list_util.exe` с нужными флагами.

Если нужен список маршрутизации для Windows или iOS, обычно лучше использовать
CIDR-диапазоны: запускайте генерацию IP через `--geoip`. Такой список
собирается из `geoip.dat` и подходит для платформ, где надёжнее маршрутизировать
трафик по IP-диапазонам.

Для Android рекомендуется использовать `--geosite`. В этом режиме утилита
создаёт список доменов из `geosite.dat`, что работает лучше для маршрутизации
на Android-клиенте Amnezia VPN. (у меня список cidr не работает вообще, подключение не устанавливается)

При добавлении списка в Amnezia VPN важно выбрать правильную логику
маршрутизации:

- Если список собран из российских категорий, например `ru` для `--geoip` или
  `category-ru` для `--geosite`, используйте режим "Адреса из списка не должны
  открываться через VPN". Так российские ресурсы будут идти напрямую, а
  остальной трафик сможет идти через VPN.
- Если список собран из конкретных категорий ресурсов, заблокированных в
  России, используйте режим "Только адреса из списка должны открываться через
  VPN". Так через VPN пойдут только адреса из подготовленного списка.

### Как импортировать список в Amnezia VPN

1. Сначала сгенерируйте JSON-файл и сохраните его на устройстве, где установлен
   Amnezia VPN.
2. Откройте приложение Amnezia VPN.
3. Перейдите в раздел "Раздельное туннелирование сайтов".
4. Нажмите на три точки в правой нижней части экрана.
5. Выберите "Импорт".
6. Рекомендуется выбрать "Заменить список с сайтами", чтобы использовать новый
   список целиком и не смешивать его со старыми правилами.
7. Выберите заранее сохранённый JSON-файл.
8. В меню проводника операционной системы найдите этот файл и подтвердите
   импорт.
9. После импорта не забудьте включить раздельное туннелирование переключателем
   в правой верхней части экрана.

После этого список доменов или CIDR-диапазонов будет добавлен в настройки
раздельного туннелирования.

Категории по умолчанию:

- `ru` для `--geoip`
- `category-ru` для `--geosite`

Другие категории можно посмотреть так:

```powershell
./routing_list_util.exe --geoip --list-categories
```

Запустить небольшой интерактивный интерфейс:

```powershell
./routing_list_util.exe
```

Если просто нажимать Enter, будут выбраны настройки по умолчанию: режим
`--geoip`, категория `ru`, файл `cidr_list.json`, формат Amnezia
`ip-list.json`. Если выбрать `--geosite`, категория по умолчанию будет
`category-ru`, а файл по умолчанию будет `domain_list.json`.

Сгенерировать файл без интерактивного интерфейса:

```powershell
./routing_list_util.exe --geoip
```

Если передан хотя бы один флаг, интерактивный интерфейс не запускается. В таком
режиме нужно явно выбрать `--geoip` или `--geosite`.

По умолчанию `geoip.dat` и `geosite.dat` не сохраняются на диск. Утилита
скачивает выбранный `.dat` в память, читает его и записывает только итоговый
JSON-файл в путь из `--out`.

Сгенерировать список доменов из `geosite.dat`:

```powershell
./routing_list_util.exe --geosite --category category-ru --out site_list.json
```

Выбрать категории:

```powershell
./routing_list_util.exe --geoip --category ru --category ru-whitelist --out cidr_list.json
```

Также можно передать несколько категорий через запятую:

```powershell
./routing_list_util.exe --geoip --category ru,ru-whitelist --out cidr_list.json
```

Утилита принимает имена категорий в обычном формате, а также с префиксами
`geoip:` и `geosite:`, например `telegram`, `geoip:telegram` или
`geosite:telegram`.

По умолчанию результат записывается в формате Amnezia `ip-list.json`:

```json
[
  {
    "hostname": "1.2.3.0/24",
    "ip": ""
  }
]
```

Чтобы записать объект с метаданными и массивом `ips` для `--geoip` или
`domains` для `--geosite`, используйте:

```powershell
./routing_list_util.exe --geoip --format metadata --out amnezia-ru-cidr.json
```

Это создаст:

```json
{
  "name": "Russian IP ranges for Amnezia VPN",
  "generated": "2026-06-22T00:00:00Z",
  "source": "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat",
  "categories": ["ru"],
  "ips": ["1.2.3.0/24"]
}
```

Чтобы записать только простой JSON-массив, используйте:

```powershell
./routing_list_util.exe --geoip --format raw-array --out amnezia-ru-cidr.json
```

Доступные форматы вывода:

- `amnezia`: объект для Amnezia `ip-list.json`
- `metadata`: объект с метаданными и массивом `ips`
- `raw-array`: простой JSON-массив

### Флаги

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--geoip` | `false` | Запустить режим генерации списка IP из `geoip.dat`. Обязателен при неинтерактивном запуске. |
| `--geosite` | `false` | Запустить режим генерации списка доменов из `geosite.dat`. Обязателен при неинтерактивном запуске. |
| `--url` | зависит от режима | URL файла `.dat` для скачивания. Для `--geoip` используется `https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat`, для `--geosite` используется `https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat`. |
| `--out` | зависит от режима | Путь к выходному JSON-файлу. Для `--geoip` используется `cidr_list.json`, для `--geosite` используется `domain_list.json`. |
| `--dat` | пусто | Читать `.dat` из локального файла вместо скачивания. |
| `--format` | `amnezia` | Формат вывода: `amnezia`, `metadata` или `raw-array`. |
| `--list-categories` | `false` | Напечатать категории из выбранного `.dat` и завершить работу. |
| `--category` | зависит от режима | Категория для включения. Для `--geoip` используется `ru`, для `--geosite` используется `category-ru`. Флаг можно повторять или передавать значения через запятую. |
| `--exclude-private-nets` | `false` | Исключить приватные сети из результата. Применяется только к `--geoip`. |
| `--exclude-ipv6` | `false` | Исключить IPv6-диапазоны из результата. Применяется только к `--geoip`. |

Пример с локальным `geoip.dat`:

```powershell
./routing_list_util.exe --geoip --dat geoip.dat --category geoip:telegram --format raw-array --out telegram-cidr.json
```

Пример без IPv6 и приватных сетей:

```powershell
./routing_list_util.exe --geoip --exclude-ipv6 --exclude-private-nets
```

Пример с локальным `geosite.dat`:

```powershell
./routing_list_util.exe --geosite --dat geosite.dat --category geosite:telegram --out site_list.json
```

## English

Small Go utility for preparing routing lists for split tunneling in Amnezia VPN.
It downloads `geoip.dat` or `geosite.dat` from
`runetfreedom/russia-v2ray-rules-dat`, extracts selected categories, and writes
CIDR ranges or domains to JSON that can be used in Amnezia routing settings.

The utility is run from the command line: open a terminal in the directory with
the executable and run `./routing_list_util.exe` with the needed flags.

For Windows and iOS routing lists, CIDR ranges are usually the better choice:
generate an IP list with `--geoip`. This uses `geoip.dat` and is a good fit for
platforms where routing by IP ranges is more reliable.

For Android, `--geosite` is recommended. This mode creates a domain list from
`geosite.dat`, which is better suited for domain-based routing in the Amnezia
VPN Android client.

When adding the list to Amnezia VPN, choose the routing logic based on what the
list contains:

- If the list is built from Russian categories, such as `ru` for `--geoip` or
  `category-ru` for `--geosite`, use "Addresses from the list should not be
  opened through VPN". This lets Russian resources go directly while the rest of
  the traffic can use the VPN.
- If the list is built from specific categories of resources blocked in Russia,
  use "Only addresses from the list should be opened through VPN". This sends
  only the prepared list through the VPN.

### How to import the list into Amnezia VPN

1. Generate the JSON file first and save it on the device where Amnezia VPN is
   installed.
2. Open the Amnezia VPN app.
3. Go to "Split tunneling for sites".
4. Tap the three-dot menu in the lower-right part of the screen.
5. Select "Import".
6. It is recommended to choose "Replace the list of sites" so the new list is
   used as a whole and is not mixed with older rules.
7. Select the JSON file you saved earlier.
8. In the operating system file picker, find that file and confirm the import.
9. After importing, remember to enable split tunneling using the toggle in the
   upper-right part of the screen.

After that, the domain or CIDR list will be added to the split tunneling
settings.

Default categories:

- `ru` for `--geoip`
- `category-ru` for `--geosite`

Other categories can be inspected with:

```powershell
./routing_list_util.exe --geoip --list-categories
```

Start the small interactive interface:

```powershell
./routing_list_util.exe
```

If you keep pressing Enter, the default options are used: `--geoip` mode, `ru`
category, `cidr_list.json` output file, and Amnezia `ip-list.json` format. If
you choose `--geosite`, the default category is `category-ru`, and the default
output file is `domain_list.json`.

Generate the file without the interactive interface:

```powershell
./routing_list_util.exe --geoip
```

If at least one flag is passed, the interactive interface is not started. In
that mode, you must explicitly choose either `--geoip` or `--geosite`.

By default, `geoip.dat` and `geosite.dat` are not saved to disk. The utility
downloads the selected `.dat` file into memory, reads it, and writes only the
final JSON file to the path from `--out`.

Generate a domain list from `geosite.dat`:

```powershell
./routing_list_util.exe --geosite --category category-ru --out site_list.json
```

Select categories:

```powershell
./routing_list_util.exe --geoip --category ru --category ru-whitelist --out cidr_list.json
```

You can also pass several categories as a comma-separated value:

```powershell
./routing_list_util.exe --geoip --category ru,ru-whitelist --out cidr_list.json
```

The utility accepts plain category names and names prefixed with `geoip:` or
`geosite:`, for example `telegram`, `geoip:telegram`, or `geosite:telegram`.

By default the output is Amnezia `ip-list.json` format:

```json
[
  {
    "hostname": "1.2.3.0/24",
    "ip": ""
  }
]
```

To write an object with metadata and an `ips` array for `--geoip` or a
`domains` array for `--geosite`, use:

```powershell
./routing_list_util.exe --geoip --format metadata --out amnezia-ru-cidr.json
```

That produces:

```json
{
  "name": "Russian IP ranges for Amnezia VPN",
  "generated": "2026-06-22T00:00:00Z",
  "source": "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat",
  "categories": ["ru"],
  "ips": ["1.2.3.0/24"]
}
```

To write only a plain JSON array, use:

```powershell
./routing_list_util.exe --geoip --format raw-array --out amnezia-ru-cidr.json
```

Available output formats:

- `amnezia`: Amnezia `ip-list.json` object
- `metadata`: object with metadata and `ips` array
- `raw-array`: plain JSON array

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--geoip` | `false` | Run the IP-list generation mode from `geoip.dat`. Required for non-interactive runs. |
| `--geosite` | `false` | Run the domain-list generation mode from `geosite.dat`. Required for non-interactive runs. |
| `--url` | depends on mode | URL of the `.dat` file to download. `--geoip` uses `https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat`; `--geosite` uses `https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat`. |
| `--out` | depends on mode | Output JSON path. `--geoip` uses `cidr_list.json`; `--geosite` uses `domain_list.json`. |
| `--dat` | empty | Read a `.dat` file from disk instead of downloading it. |
| `--format` | `amnezia` | Output format: `amnezia`, `metadata`, or `raw-array`. |
| `--list-categories` | `false` | Print categories found in the selected `.dat` file and exit. |
| `--category` | depends on mode | Category to include. `--geoip` uses `ru`; `--geosite` uses `category-ru`. Repeat the flag or pass comma-separated values. |
| `--exclude-private-nets` | `false` | Exclude private IP ranges from the output. Applies only to `--geoip`. |
| `--exclude-ipv6` | `false` | Exclude IPv6 ranges from the output. Applies only to `--geoip`. |

Example with a local `geoip.dat`:

```powershell
./routing_list_util.exe --geoip --dat geoip.dat --category geoip:telegram --format raw-array --out telegram-cidr.json
```

Example without IPv6 and private networks:

```powershell
./routing_list_util.exe --geoip --exclude-ipv6 --exclude-private-nets
```

Example with a local `geosite.dat`:

```powershell
./routing_list_util.exe --geosite --dat geosite.dat --category geosite:telegram --out site_list.json
```
