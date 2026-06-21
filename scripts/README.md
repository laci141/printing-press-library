# Weekly NEJM Report Scripts

These PowerShell scripts help you generate a weekly summary of NEJM articles using the `nejm-pp-cli` tool.

## Prerequisites
- Windows 10/11 with PowerShell 5.1+
- `nejm-pp-cli.exe` installed and available in your PATH (or placed in the same folder as the scripts)

## Quick Start
1. Open PowerShell in this folder.
2. Run the full process:
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\run_weekly.ps1