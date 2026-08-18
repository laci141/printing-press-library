# Csak a heti cikkek gyűjtése
$doik = .\nejm-pp-cli.exe sql "SELECT doi FROM article WHERE date >= date('now', '-7 days')" | Select-Object -Skip 1 | Where-Object { $_.Trim() -ne "" }
$osszes = $doik.Count
$i = 0
$eredmenyek = @()

Write-Host ""
Write-Host "HETI CIKKEK GYUJTESE" -ForegroundColor Cyan
Write-Host "====================="
Write-Host "Osszes heti cikk: $osszes db"
Write-Host ""

foreach ($doi in $doik) {
    $i++
    $doi = $doi.Trim()
    if ([string]::IsNullOrEmpty($doi)) { continue }
    
    Write-Host "[$i / $osszes] $doi" -ForegroundColor Yellow
    
    $jsonOutput = .\nejm-pp-cli.exe --json article $doi --enrich 2>$null
    
    if ($LASTEXITCODE -eq 0 -and $jsonOutput) {
        try {
            $cikkData = $jsonOutput | ConvertFrom-Json
            $cikk = [PSCustomObject]@{
                doi = $doi
                title = $cikkData.title
                date = $cikkData.date
                article_type = $cikkData.article_type
                specialties = $cikkData.specialties
                is_free = $cikkData.is_free
            }
            Write-Host "  $($cikk.title)" -ForegroundColor Green
            $eredmenyek += $cikk
        } catch {
            Write-Host "  Hiba a JSON feldolgozas soran" -ForegroundColor Red
        }
    } else {
        Write-Host "  Hiba a lekérés soran" -ForegroundColor Red
    }
    Start-Sleep -Milliseconds 500
}

$eredmenyek | ConvertTo-Json -Depth 3 | Out-File -FilePath .\kiegeszitett_cikkek.json -Encoding UTF8
Write-Host ""
Write-Host "================================"
Write-Host "GYUJTES BEFEJEZVE" -ForegroundColor Cyan
Write-Host "$osszes cikk feldolgozva."
Write-Host ""