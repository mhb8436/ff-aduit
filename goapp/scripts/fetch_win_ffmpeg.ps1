# Windows용 ffmpeg/ffprobe 정적 바이너리를 내려받아 내장 자산(gz)으로 준비한다.
# go:embed 대상(internal\tools\assets\windows\*.exe.gz)을 생성한다.
#   - Windows exe 빌드 전에 한 번만 실행.
#   - 실행: PowerShell 에서
#       powershell -ExecutionPolicy Bypass -File scripts\fetch_win_ffmpeg.ps1
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$assets = Join-Path $PSScriptRoot "..\internal\tools\assets\windows"
$url = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
$tmp = Join-Path $env:TEMP ("vqc-ffmpeg-" + [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $assets | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

Write-Host "[1/3] 다운로드: $url"
$zip = Join-Path $tmp "ffmpeg.zip"
Invoke-WebRequest -Uri $url -OutFile $zip

Write-Host "[2/3] 압축 해제"
Expand-Archive -Path $zip -DestinationPath $tmp -Force
$ffmpeg  = Get-ChildItem -Path $tmp -Recurse -Filter ffmpeg.exe  | Select-Object -First 1
$ffprobe = Get-ChildItem -Path $tmp -Recurse -Filter ffprobe.exe | Select-Object -First 1
if (-not $ffmpeg -or -not $ffprobe) { throw "ffmpeg.exe / ffprobe.exe 를 찾지 못했습니다." }

function Compress-Gzip($src, $dst) {
    $in  = [System.IO.File]::OpenRead($src)
    $out = [System.IO.File]::Create($dst)
    $gz  = New-Object System.IO.Compression.GZipStream($out, [System.IO.Compression.CompressionMode]::Compress)
    $in.CopyTo($gz)
    $gz.Dispose(); $out.Dispose(); $in.Dispose()
}

Write-Host "[3/3] gzip 내장 자산 생성"
Compress-Gzip $ffmpeg.FullName  (Join-Path $assets "ffmpeg.exe.gz")
Compress-Gzip $ffprobe.FullName (Join-Path $assets "ffprobe.exe.gz")
Remove-Item -Recurse -Force $tmp

Write-Host "완료. 이제 goapp 에서 'go build -o dist\vqc.exe .' 로 vqc.exe 를 만들 수 있습니다."
