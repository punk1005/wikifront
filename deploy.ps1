# --- Настройки подключения ---
$serverIp   = "192.168.1.101"
$sshPort    = "2205"
$sshUser    = "alexpunk" # Поменяйте на вашего пользователя, если это не root
$remotePath = "/usr/local/sbin/cicd/scripts/services/wikifront/"

# --- Локальные файлы ---
$binaryFile = "wikifront"
$envFile    = ".env"
$staticFolder = ".\static"

# --- АВТОМАТИЧЕСКИЙ СЧЕТЧИК ВЕРСИИ ---
Write-Host "--- Расчет версии проекта ---" -ForegroundColor Cyan
if (-not (Test-Path $envFile)) {
    Write-Error "Ошибка: Файл '$envFile' не найден! Создайте его с переменной PROJECT_VERSION=1.0.0.0"
    exit 1
}

# 1. Читаем текущую версию из .env
$envContent = Get-Content $envFile -Raw
$versionMatch = [regex]::Match($envContent, 'PROJECT_VERSION=(?<major>\d+)\.(?<minor>\d+)\.(?<date>\d+)\.(?<build>\d+)')

if ($versionMatch.Success) {
    $major = $versionMatch.Groups['major'].Value
    $minor = $versionMatch.Groups['minor'].Value
    $oldDate = $versionMatch.Groups['date'].Value
    $oldBuild = [int]$versionMatch.Groups['build'].Value

    # Получаем текущую дату в формате ГГММДД (например, 260825)
    $currentDate = Get-Date -Format "yyMMdd"

    # Если дата совпадает со старой — увеличиваем счетчик сборок. Если день новый — сбрасываем в 1.
    if ($oldDate -eq $currentDate) {
        $newBuild = $oldBuild + 1
    } else {
        $newBuild = 1
    }
    
    $newVersion = "$major.$minor.$currentDate.$newBuild"
} else {
    # Если в .env версия была в старом формате (например 0.0.0), инициализируем новый формат
    $currentDate = Get-Date -Format "yyMMdd"
    $newVersion = "1.0.$currentDate.1"
    Write-Host "Формат версии обновлен на новый автоматический." -ForegroundColor Yellow
}

# 2. Обновляем .env файл локально
if ($versionMatch.Success) {
    $envContent = $envContent -replace 'PROJECT_VERSION=\d+\.\d+\.\d+\.\d+', "PROJECT_VERSION=$newVersion"
} else {
    # Если строки вообще не было или она была другой структуры (0.0.0)
    if ($envContent -match 'PROJECT_VERSION=.*') {
        $envContent = $envContent -replace 'PROJECT_VERSION=.*', "PROJECT_VERSION=$newVersion"
    } else {
        $envContent += "`nPROJECT_VERSION=$newVersion"
    }
}
Set-Content $envFile $envContent -NoNewline
Write-Host "Новая версия проекта: $newVersion" -ForegroundColor Green


# --- Сборка Go-приложения под Linux ---
Write-Host "0. Компиляция Go-приложения под Linux..." -ForegroundColor Cyan

# Устанавливаем переменные окружения для кросс-компиляции
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

# Запускаем сборку
go build -ldflags="-s -w" -o $binaryFile . 

# --- Проверка наличия локальных файлов перед отправкой ---
if (-not (Test-Path $binaryFile)) {
    Write-Error "Ошибка: Бинарник '$binaryFile' не найден в текущей директории!"
    exit 1
}
if (-not (Test-Path $envFile)) {
    Write-Error "Ошибка: Файл '$envFile' не найден в текущей директории!"
    exit 1
}
if (-not (Test-Path $staticFolder -PathType Container)) {
    Write-Error "Ошибка: Папка '$staticFolder' не найдена!"
    exit 1
}

# Принудительно удаляем контейнер до отправки файлов, чтобы освободить бинарник task1
Write-Host "0. Остановка старого контейнера на сервере..." -ForegroundColor Cyan
ssh -p $sshPort "${sshUser}@${serverIp}" "sudo docker rm -f punk-$($binaryFile)" 2>$null


Write-Host "1. Отправка файлов на сервер $serverIp..." -ForegroundColor Cyan

# Отправляем бинарник и .env одной командой scp
# Аргумент -P указывает кастомный порт SSH
scp -P $sshPort -r $binaryFile $envFile $staticFolder "${sshUser}@${serverIp}:${remotePath}"

if ($LASTEXITCODE -ne 0) {
    Write-Error "Ошибка при передаче файлов по SCP!"
    exit 1
}

Write-Host "2. Файлы успешно загружены. Запуск reload.sh на сервере..." -ForegroundColor Cyan

# Подключаемся по SSH, переходим в папку, даем права на исполнение бинарнику (на всякий случай) и запускаем reload.sh
ssh -p $sshPort "${sshUser}@${serverIp}" "cd $remotePath && chmod +x $binaryFile && sudo ./reload.sh"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Деплой и перезапуск успешно завершены!" -ForegroundColor Green
} else {
    Write-Error "Скрипт reload.sh завершился с ошибкой на удаленном сервере."
}