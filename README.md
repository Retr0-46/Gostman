# Gostman

Небольшой CLI для отправки HTTP-запросов. Команда `send` показывает исходный и конечный URL, статус, протокол, время выполнения, размер тела, все заголовки ответа и само тело. Редиректы обрабатываются стандартным HTTP-клиентом Go.

## Запуск в PowerShell

Нужен Go 1.25.6 или новее. Из корня репозитория команду можно запустить без установки:

```powershell
go run ./cmd/cli send https://example.com -X GET
```

Чтобы собрать `gostman.exe`:

```powershell
go build -o gostman.exe ./cmd/cli
.\gostman.exe send https://example.com -X GET
```

PowerShell не запускает программы из текущей папки по одному имени: нужен префикс `.\`. Для вызова `gostman` из любой папки добавьте папку с `gostman.exe` в `PATH`.

В текущем окне PowerShell это можно сделать так (находясь в корне репозитория):

```powershell
$env:Path = "$PWD;$env:Path"
gostman send https://example.com -X GET
```

## Примеры

```powershell
# GET с заголовками
.\gostman.exe send https://example.com -H "Accept: text/html" -H "X-Trace: demo"

# POST с JSON-телом (Windows PowerShell 5.1)
.\gostman.exe send https://httpbin.org/post -X POST -H "Content-Type: application/json" -d '{\"name\":\"test\"}'

# Тело из файла
.\gostman.exe send https://httpbin.org/post -X POST -H "Content-Type: application/json" --data-file .\body.json

# Файл как часть multipart/form-data вместе с обычным полем
.\gostman.exe send https://httpbin.org/post -X POST -F 'description=avatar' -F 'file=@.\photo.jpg'

# Тело из стандартного ввода
Get-Content .\body.json -Raw | .\gostman.exe send https://httpbin.org/post -X POST --data-file -

# Таймаут
.\gostman.exe send https://example.com --timeout 10s
```

`-F, --form` можно повторять: `-F 'name=value'` добавляет текстовое поле, `-F 'file=@.\photo.jpg'` добавляет файл. Имя файла берётся из пути, MIME-тип — из расширения (если тип неизвестен, используется `application/octet-stream`). Заголовок `Content-Type: multipart/form-data` с boundary создаётся автоматически, вручную его указывать не нужно. Для текстового значения, начинающегося с `@`, используйте `@@`: `-F 'handle=@@user'`. `-F` нельзя сочетать с `-d` или `--data-file`; файл читается целиком в память один раз перед отправкой.

## Параметризованные запросы

`--match` принимает **один аргумент** вида `'{name}=(value1, value2, value3)'`. Значения последовательно подставляются вместо `{name}` в URL, заголовки и тело запроса. Для нескольких `--match` выполняются все сочетания значений. Запросы отправляются по одному, максимум 100 за запуск.

```powershell
# Три GET-запроса к /1, /2 и /3
.\gostman.exe send 'https://httpbin.org/anything/{value}' --match '{value}=(1, 2, 3)'

# Три POST-запроса; поле id в JSON будет числом
.\gostman.exe send https://httpbin.org/post -X POST -H "Content-Type: application/json" -d '{\"id\":\"{value}\"}' --match '{value}=(1, 2, 3)'

# Все сочетания двух параметров: 2 x 2 = 4 запроса
.\gostman.exe send 'https://httpbin.org/anything/{group}/{id}' --match '{group}=(a, b)' --match '{id}=(1, 2)'

# Подстановка в текстовое поле multipart; байты файла не изменяются
.\gostman.exe send 'https://httpbin.org/post' -X POST -F 'label={value}' -F 'file=@.\photo.jpg' --match '{value}=(one, two)'
```

Список значений разделяется запятыми; для значения с запятой используйте CSV-кавычки: `--match 'name=("red,blue", green)'`. В JSON шаблон тела должен быть корректным JSON до подстановки: `{"id":"{value}"}`. Если вся строка равна `"{value}"`, числа, `true`, `false` и `null` становятся соответствующими JSON-значениями; остальные значения остаются строками. Внутри более длинной строки подстановка всегда текстовая. Шаблоны работают и с `--data-file`.

Для каждого варианта выводится полный ответ, в конце — число запросов и сетевых ошибок. Если один запрос не удался, остальные всё равно выполняются; код завершения будет 1. HTTP 4xx/5xx по-прежнему считаются полученными ответами.

## Нагрузка на API

`-rps` (или `--rps`) задаёт общий темп отправки запросов в секунду, `-c` (или `--clients`) — число одновременных клиентов. Режим работает в течение `--duration` (по умолчанию 10 секунд). Если все клиенты заняты, очередной запрос пропускается; накопления очереди и последующего всплеска нет. Варианты `--match` отправляются по кругу.

```powershell
.\gostman.exe send http://127.0.0.1:8080/api -rps 20 -c 4 --duration 10s --expect-status 200

# Удалённый тестовый стенд
.\gostman.exe send https://api.test.example.com/health -rps 10 -c 2 --duration 10s --expect-status 200
```

В этом режиме выводится сводка: сколько запросов выполнено или пропущено, фактическая пропускная способность, коды ответов, задержка и число успешных/проваленных проверок. Тела ответов по отдельности не печатаются; перенаправления не выполняются. Для `localhost` и loopback-адресов пределы составляют 100 запросов/с, 20 клиентов, 30 секунд. Для внешних адресов — 10 запросов/с, 2 клиента, 10 секунд. Код завершения 1 означает сетевую ошибку или проваленную проверку.

## Проверка ответов

`--expect-status` проверяет HTTP-статус, а `--expect-json` — поле в JSON-ответе. Оба флага можно повторять. Несовпадение помечается `FAIL` и завершает команду с кодом 1 после выполнения всей серии.

```powershell
# Один ожидаемый статус для всех запросов
.\gostman.exe send https://httpbin.org/get --expect-status 200

# Для двух вариантов ожидаются разные статусы по порядку случаев
.\gostman.exe send 'https://httpbin.org/status/{code}' --match '{code}=(200, 404)' --expect-status '(200, 404)'

# Ожидаемый статус также может ссылаться на значение --match
.\gostman.exe send 'https://httpbin.org/status/{code}' --match '{code}=(200, 404)' --expect-status '{code}'

# Два --match: text используется в запросе, code — только в ожидаемом статусе
.\gostman.exe send https://httpbin.org/post -X POST -H "Content-Type: application/json" -d '{\"name\":\"{text}\"}' --match '{text}=(dfdf)' --match '{code}=(200)' --expect-status '{code}' --expect-json '/json/name={text}'

# Проверка заголовка через прежний --expect
.\gostman.exe send https://httpbin.org/get --expect 'header:Content-Type=application/json'
```

`--expect-json 'id=123'` обращается к полю `id` в корне ответа. Для вложенных полей используйте путь с `/`, например `--expect-json '/user/id=42'` или `--expect-json '/items/0/name=Alice'`. Сравнение учитывает тип: `42`, `true` и `null` — значения JSON, остальные некавыченные слова — строки. В списке `--expect-status '(...)'` число ожидаемых значений должно совпадать с числом вариантов запроса; порядок совпадает с порядком `Case` в выводе. Параметр `--match` может использоваться только в ожидании. Ошибки в ожиданиях проверяются до отправки запросов. HTTP 4xx/5xx проходят тест, если их статус соответствует ожиданию. Старый `--expect 'status=200'` и `--expect 'json:/id=123'` также поддерживаются.

`-X, --method` задаёт метод (по умолчанию `GET`). `-H, --header` можно повторять; разделитель имени и значения заголовка — первое двоеточие. `-d, --data` и `--data-file` взаимоисключающие. Таймаут по умолчанию составляет 30 секунд. Список флагов: `.\gostman.exe send --help`.

В Windows PowerShell 5.1 при передаче JSON через `-d` внутренние двойные кавычки нужно экранировать обратной косой чертой, как в примере выше. Для сложных тел удобнее `--data-file`.

Ответы HTTP 4xx/5xx показываются как обычные ответы вместе с телом. Ошибки сети, неверный URL, ошибки чтения файла и таймаут завершают команду с кодом 1. Тело ответа выводится как есть, поэтому для бинарных ответов терминал может отображать нечитаемые символы.

Если запрос к конкретному сайту возвращает `EOF`, проверьте тот же URL в браузере или другом HTTP-клиенте: соединение мог закрыть удалённый сервер или промежуточный узел. Для проверки самого запуска CLI используйте пример с `https://example.com`.
