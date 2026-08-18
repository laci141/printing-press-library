# NEJM adatbázis kiegészítése az article --enrich segítségével
# A szkript végigmegy az összes DOI-n, és frissíti a hiányzó mezőket

$doik = .\nejm-pp-cli.exe sql "SELECT doi FROM article" | Select-Object -Skip 1 | Where-Object { $_.Trim() -ne "" }
$osszes = $doik.Count
$i = 0
$hibak = 0

Write-Host ""
Write-Host "NEJM ADATKIEESZITES INDULASA" -ForegroundColor Cyan
Write-Host "================================"
Write-Host "Osszes cikk: $osszes db"
Write-Host ""

foreach ($doi in $doik) {
    $i++
    $doi = $doi.Trim()
    if ([string]::IsNullOrEmpty($doi)) { continue }
    
    Write-Host "[$i / $osszes] $doi" -ForegroundColor Yellow
    
    # Az article --enrich kimenetét eltároljuk
    $output = .\nejm-pp-cli.exe article $doi --enrich 2>&1
    
    # Ellenőrizzük, hogy sikerült-e a lekérés
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  Hiba: a lekérés sikertelen" -ForegroundColor Red
        $hibak++
        Start-Sleep -Milliseconds 500
        continue
    }
    
    # Kinyerjük az article_type értéket
    $type = ""
    if ($output -match "article_type:\s*(.+?)(\r?\n|$)") {
        $type = $matches[1].Trim()
    }
    
    # Kinyerjük a specialties értéket
    $specialties = ""
    if ($output -match "specialties:\s*(.+?)(\r?\n|$)") {
        $specialties = $matches[1].Trim()
    }
    
    # Kinyerjük az is_free értéket
    $is_free = "0"
    if ($output -match "is_free:\s*(.+?)(\r?\n|$)") {
        $is_free = $matches[1].Trim()
        if ($is_free -eq "true" -or $is_free -eq "True" -or $is_free -eq "1") {
            $is_free = "1"
        } else {
            $is_free = "0"
        }
    }
    
    # Frissítjük az adatbázist
    if ($type -ne "" -or $specialties -ne "" -or $is_free -ne "0") {
        $updateSql = "UPDATE article SET article_type = '$type', specialties = '$specialties', is_free = $is_free WHERE doi = '$doi'"
        $updateResult = .\nejm-pp-cli.exe sql $updateSql 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            Write-Host "  Frissitve: type='$type', specialties='$specialties', free=$is_free" -ForegroundColor Green
        } else {
            Write-Host "  Hiba a frissites soran" -ForegroundColor Red
            $hibak++
        }
    } else {
        Write-Host "  Nincs uj adat" -ForegroundColor Gray
    }
    
    # Várunk 0.5 másodpercet a következő hívás előtt
    Start-Sleep -Milliseconds 500
}

Write-Host ""
Write-Host "================================"
Write-Host "KIEESZITES BEFEJEZVE" -ForegroundColor Cyan
Write-Host "Osszes cikk: $osszes"
Write-Host "Sikeres: $($osszes - $hibak)"
Write-Host "Hibak: $hibak"
Write-Host ""