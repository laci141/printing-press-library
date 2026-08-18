# Heti NEJM elemzes magyar nyelven, ekezetek nelkul, de a kimenet jol olvashato
$json = .\nejm-pp-cli.exe --json sql "SELECT * FROM article WHERE date >= date('now', '-7 days')" | ConvertFrom-Json
$cikkek = $json.results

if ($cikkek.Count -eq 0) {
    Write-Host "Nincs cikk az elmult 7 napbol."
    exit
}

Write-Host ""
Write-Host "HETI NEJM OSSZESITES (az elmult 7 nap)"
Write-Host "============================================"
Write-Host ""
Write-Host "Osszes cikk: $($cikkek.Count) db"

$tipusok = $cikkek | Group-Object article_type | Sort-Object Count -Descending
Write-Host ""
Write-Host "Bontas tipus szerint:"
foreach ($t in $tipusok) {
    Write-Host "   $($t.Name): $($t.Count) db"
}

$legfrissebb = $cikkek | Sort-Object date -Descending | Select-Object -First 5
Write-Host ""
Write-Host "Legfrissebb 5 cikk:"
$i = 1
foreach ($c in $legfrissebb) {
    $datum = $c.date
    if ($datum.Length -ge 10) { $datum = $datum.Substring(0,10) }
    Write-Host "   $i. $($c.title) ($datum)"
    $i++
}

$specialties = $cikkek | Where-Object { $_.specialties -and $_.specialties -ne "" } | ForEach-Object { $_.specialties -split "," } | ForEach-Object { $_.Trim() } | Group-Object | Sort-Object Count -Descending
if ($specialties.Count -gt 0) {
    Write-Host ""
    Write-Host "Leggyakoribb szakteruletek:"
    $specialties | Select-Object -First 5 | ForEach-Object {
        Write-Host "   $($_.Name): $($_.Count) db"
    }
} else {
    Write-Host ""
    Write-Host "Nincs szakteruleti adat a cikkekben."
}

$free = ($cikkek | Where-Object { $_.is_free -eq 1 }).Count
Write-Host ""
Write-Host "Nyilt hozzaferesu cikkek: $free db"
Write-Host ""
Write-Host "Elemzes kesz!"