# weekly_fetch.ps1
# Fetches all articles from the last 7 days and enriches them with metadata (type, specialties, open-access).

$dois = .\nejm-pp-cli.exe sql "SELECT doi FROM article WHERE date >= date('now', '-7 days')" | Select-Object -Skip 1 | Where-Object { $_.Trim() -ne "" }
$total = $dois.Count
$i = 0
$results = @()

Write-Host ""
Write-Host "FETCHING WEEKLY ARTICLES" -ForegroundColor Cyan
Write-Host "========================"
Write-Host "Total articles: $total"
Write-Host ""

foreach ($doi in $dois) {
    $i++
    $doi = $doi.Trim()
    if ([string]::IsNullOrEmpty($doi)) { continue }
    
    Write-Host "[$i / $total] $doi" -ForegroundColor Yellow
    
    $jsonOutput = .\nejm-pp-cli.exe --json article $doi --enrich 2>$null
    
    if ($LASTEXITCODE -eq 0 -and $jsonOutput) {
        try {
            $data = $jsonOutput | ConvertFrom-Json
            $article = [PSCustomObject]@{
                doi = $doi
                title = $data.title
                date = $data.date
                article_type = $data.article_type
                specialties = $data.specialties
                is_free = $data.is_free
            }
            Write-Host "  $($article.title)" -ForegroundColor Green
            $results += $article
        } catch {
            Write-Host "  ERROR: JSON parsing failed" -ForegroundColor Red
        }
    } else {
        Write-Host "  ERROR: API request failed" -ForegroundColor Red
    }
    Start-Sleep -Milliseconds 500
}

$results | ConvertTo-Json -Depth 3 | Out-File -FilePath .\weekly_articles.json -Encoding UTF8
Write-Host ""
Write-Host "================================"
Write-Host "FETCH COMPLETE" -ForegroundColor Cyan
Write-Host "$total articles processed."
Write-Host "Data saved to weekly_articles.json"
Write-Host ""