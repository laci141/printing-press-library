# Heti NEJM elemzes a kiegeszitett JSON adatokbol
$json = Get-Content .\kiegeszitett_cikkek.json | ConvertFrom-Json
$osszesCikk = $json

$most = Get-Date
$hetEzelott = $most.AddDays(-7)

$hetiCikkek = $osszesCikk | Where-Object {
    $d = $_.date
    if ($d -is [string]) {
        try {
            $datum = [datetime]::ParseExact($d, "yyyy-MM-ddTHH:mm:ssZ", $null)
        } catch {
            $datum = [datetime]::ParseExact($d.Substring(0,10), "yyyy-MM-dd", $null)
        }
    } else {
        $datum = $d
    }
    $datum -ge $hetEzelott
}

if ($hetiCikkek.Count -eq 0) {
    Write-Host "Nincsenek cikkek az elmult 7 napbol."
    exit
}

Write-Host ""
Write-Host "HETI NEJM OSSZESITES (az elmult 7 nap)"
Write-Host "============================================"
Write-Host ""
Write-Host "Osszes cikk: $($hetiCikkek.Count) db"

# Bontas tipus szerint
$tipusok = $hetiCikkek | Where-Object { $_.article_type -and $_.article_type -ne "" } | Group-Object article_type | Sort-Object Count -Descending
if ($tipusok.Count -gt 0) {
    Write-Host ""
    Write-Host "Bontas tipus szerint:"
    foreach ($t in $tipusok) {
        Write-Host "   $($t.Name): $($t.Count) db"
    }
} else {
    Write-Host ""
    Write-Host "Nincs tipusinformacio a cikkekben."
}

# Legfrissebb 5 cikk
$legfrissebb = $hetiCikkek | Where-Object { $_.date } | Sort-Object { [datetime]$_.date } -Descending | Select-Object -First 5
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

# Szakteruletek
$specialties = $hetiCikkek | Where-Object { $_.specialties -and $_.specialties -ne "" } | ForEach-Object { $_.specialties -split "\|" } | ForEach-Object { $_.Trim() } | Group-Object | Sort-Object Count -Descending
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

# Nyilt hozzaferesu cikkek
$free = ($hetiCikkek | Where-Object { $_.is_free -eq $true }).Count
Write-Host ""
Write-Host "Nyilt hozzaferesu cikkek: $free db"
Write-Host ""
Write-Host "Elemzes kesz!"