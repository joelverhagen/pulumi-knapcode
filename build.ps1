[CmdletBinding()]
param (
    [Parameter(Mandatory = $false)]
    [ValidateSet("Release", "Debug", IgnoreCase = $false)]
    [string]$Configuration = "Release"
)

$ErrorActionPreference = "Stop"

if ($Configuration -eq "Debug") {
    $gcflags = @(-gcflags '-N -l')
}

$artifacts = Join-Path $PSScriptRoot "artifacts"
if (!(Test-Path $artifacts)) {
    New-Item $artifacts -Type Directory | Out-Null
}

Write-Host "Building Go tools..."
go build -o $artifacts @gcflags `
    (Join-Path $PSScriptRoot "cmd\pulumi-resource-knapcode") `
    (Join-Path $PSScriptRoot "cmd\pulumi-sdkgen-knapcode")
if ($LASTEXITCODE) { throw "go build failed." }

Write-Host ""
Write-Host "Deleting current SDK..."
$sdk = (Join-Path $PSScriptRoot "sdk")
if (Test-Path $sdk) {
    Remove-Item $sdk -Recurse -Force
}

Write-Host ""
Write-Host "Generating SDK..."
$schema = Join-Path $PSScriptRoot "schema.json"
$output = "first run"
while ($output) {
    $output = & (Join-Path $artifacts "pulumi-sdkgen-knapcode") $schema $sdk 
}

Write-Host ""
Write-Host "Setting dotnet version.txt..."
$schemaJson = Get-Content $schema | ConvertFrom-Json
$version = $schemaJson.version
$version | Set-Content (Join-Path $sdk "dotnet\version.txt") -Encoding ASCII

Write-Host ""
Write-Host "Building NuGet package ..."
dotnet build (Join-Path $sdk "dotnet\Pulumi.Knapcode.csproj") `
    "/p:Version=$version" `
    "/p:PackageOutputDir=$artifacts" `
    -c $Configuration

Write-Host ""
Write-Host "Compressing provider plugin..."
tar -C $artifacts `
    -cvzf `
    (Join-Path $artifacts "pulumi-resource-knapcode-v$version-windows-amd64.tar.gz") `
    "pulumi-resource-knapcode.exe"
