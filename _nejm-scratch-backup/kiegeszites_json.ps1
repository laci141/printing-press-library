# NEJM adatgyujtes – JSON valasz feldolgozasa a --json flaggel
$doik = .\nejm-pp-cli.exe sql "SELECT doi FROM article" | Select-Object -Skip 1 | Where-Object { $_.Trim() -ne "" }
$osszes = $doik.Count
$i = 0
$eredmenyek = @()

Write-Host ""
Write-Host "NEJM ADATGYUJTES INDULASA (JSON mod)" -ForegroundColor Cyan
Write-Host "=========================================="
Write-Host "Osszes cikk: $osszes db"
Write-Host ""

foreach ($doi in $doik) {
    $i++
    $doi = $doi.Trim()
    if ([string]::IsNullOrEmpty($doi)) { continue }
    
    Write-Host "[$i / $osszes] $doi" -ForegroundColor Yellow
    
    # A --json flaggel lekérjük a JSON választ
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
            Write-Host "  Cim: $($cikk.title)" -ForegroundColor Green
            Write-Host "  Tipus: $($cikk.article_type)" -ForegroundColor Green
            Write-Host "  Szakterulet: $($cikk.specialties)" -ForegroundColor Green
            Write-Host "  Nyilt hozzaferes: $($cikk.is_free)" -ForegroundColor Green
        } catch {
            Write-Host "  Hiba a JSON feldolgozas soran" -ForegroundColor Red
            $cikk = [PSCustomObject]@{doi=$doi; title=""; date=""; article_type=""; specialties=""; is_free=$false}
        }
    } else {
        Write-Host "  Hiba a lekérés soran" -ForegroundColor Red
        $cikk = [PSCustomObject]@{doi=$doi; title=""; date=""; article_type=""; specialties=""; is_free=$false}
    }
    
    $eredmenyek += $cikk
    Start-Sleep -Milliseconds 500
}

$eredmenyek | ConvertTo-Json -Depth 3 | Out-File -FilePath .\kiegeszitett_cikkek.json -Encoding UTF8
Write-Host ""
Write-Host "================================"
Write-Host "GYUJTES BEFEJEZVE" -ForegroundColor Cyan
Write-Host "Osszes cikk: $osszes"
Write-Host "Az adatok a 'kiegeszitett_cikkek.json' fajlban talalhatok."
Write-Host ""