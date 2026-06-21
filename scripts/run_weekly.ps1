# run_weekly.ps1
# Runs the full weekly process: fetch enriched data, then generate report.

Write-Host "STARTING WEEKLY NEJM PROCESS" -ForegroundColor Magenta
Write-Host "=============================="
Write-Host ""

# Step 1: Fetch
.\weekly_fetch.ps1

# Step 2: Report
.\weekly_report.ps1

Write-Host ""
Write-Host "ALL DONE." -ForegroundColor Green