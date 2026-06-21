# weekly_report.ps1
# Generates a weekly summary from the enriched JSON data.

$json = Get-Content .\weekly_articles.json | ConvertFrom-Json
$articles = $json

if ($articles.Count -eq 0) {
    Write-Host "No articles found for the last 7 days."
    exit
}

Write-Host ""
Write-Host "WEEKLY NEJM SUMMARY (last 7 days)" -ForegroundColor Cyan
Write-Host "=================================="
Write-Host ""
Write-Host "Total articles: $($articles.Count)"

# Breakdown by type
$types = $articles | Where-Object { $_.article_type -and $_.article_type -ne "" } | Group-Object article_type | Sort-Object Count -Descending
if ($types.Count -gt 0) {
    Write-Host ""
    Write-Host "Breakdown by type:"
    foreach ($t in $types) {
        Write-Host "   $($t.Name): $($t.Count)"
    }
} else {
    Write-Host ""
    Write-Host "No type information available."
}

# Top 5 newest articles
$latest = $articles | Where-Object { $_.date } | Sort-Object { [datetime]$_.date } -Descending | Select-Object -First 5
Write-Host ""
Write-Host "Top 5 newest articles:"
$i = 1
foreach ($a in $latest) {
    $date = $a.date
    if ($date.Length -ge 10) { $date = $date.Substring(0,10) }
    Write-Host "   $i. $($a.title) ($date)"
    $i++
}

# Top specialties
$specialties = $articles | Where-Object { $_.specialties -and $_.specialties -ne "" } | ForEach-Object { $_.specialties -split "\|" } | ForEach-Object { $_.Trim() } | Group-Object | Sort-Object Count -Descending
if ($specialties.Count -gt 0) {
    Write-Host ""
    Write-Host "Top specialties:"
    $specialties | Select-Object -First 5 | ForEach-Object {
        Write-Host "   $($_.Name): $($_.Count)"
    }
} else {
    Write-Host ""
    Write-Host "No specialty data available."
}

# Open access
$free = ($articles | Where-Object { $_.is_free -eq $true }).Count
Write-Host ""
Write-Host "Open-access articles: $free"
Write-Host ""
Write-Host "Report complete."