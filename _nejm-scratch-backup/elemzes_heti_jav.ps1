# Heti NEJM elemzes - tabulatorral tagolt CSV feldolgozasa
$raw = Get-Content .\heti_szoveg.txt -Raw

# Az elso sor a fejlec, a tobbi az adat
$lines = $raw -split "`r?`n"
$header = $lines[0]
$dataLines = $lines[1..($lines.Count-1)] | Where-Object { $_.Trim() -ne "" }

# A fejlecbol kiszedjuk az oszlopneveket (tabulatorral tagolva)
$headers = $header -split "`t"

# CSV szoveg letrehozasa: fejlec + adatsorok (tabulatorral elvalasztva)
$csvText = $header + "`n" + ($dataLines -join "`n")

# CSV feldolgozasa tabulator elvalasztoval
$cikkek = $csvText | ConvertFrom-Csv -Delimiter "`t"

if ($cikkek.Count -eq 0) {
    Write-Host "Nincsenek cikkek az elmult 7 napbol."
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
    $nev = if ($t.Name) { $t.Name } else { "Ismeretlen" }
    Write-Host "   $nev : $($t.Count) db"
}

$legfrissebb = $cikkek | Where-Object { $_.date } | Sort-Object date -Descending | Select-Object -First 5
Write-Host ""
Write-Host "Legfrissebb 5 cikk:"
$i = 1
foreach ($c in $legfrissebb) {
    $datum = $c.date
    if ($datum.Length -ge 10) { $datum = $datum.Substring(0,10) }
    $cim = if ($c.title) { $c.title } else { "Nincs cim" }
    Write-Host "   $i. $cim ($datum)"
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