# Exercise the publish transport without touching any real tag or remote.
$ErrorActionPreference = 'Stop'
$root = Join-Path $env:TEMP ('catalog-publish-test-' + [Guid]::NewGuid())
$null = New-Item -ItemType Directory -Path $root
$transaction = Join-Path $root 'catalog-publish-transaction.json'
$version = 'v999.0.0'
$ref = "refs/tags/$version"
$tag = '1' * 40
$head = '2' * 40
$newer = '3' * 40
$privateUrl = 'https://git.lan.orcl.com.br/marcuoli/go-oracle-info.git'
$publicUrl = 'https://github.com/marcuoli/go-oracle-info.git'
$pushes = [System.Collections.Generic.List[string]]::new()
if (Test-Path Variable:global:goOracleInfoPublishFixture) { throw 'Mock already active' }
$global:goOracleInfoPublishFixture = @{tag=$tag; ref=$ref; head=$head; newer=$newer; privateTag=''; publicTag=''; privateUrl=$privateUrl; publicUrl=$publicUrl; pushes=$pushes}

function Test-Python {
    # The Python verifier itself is exercised independently. This stub restricts
    # the mock transport to an existing synthetic transaction.
    $global:LASTEXITCODE = 1
    if ($args -contains 'resume' -and (Test-Path -LiteralPath $transaction)) {
        $global:LASTEXITCODE = 0
    }
}

function git {
    $f = $global:goOracleInfoPublishFixture
    $global:LASTEXITCODE = 0
    $call = @($args)
    switch ($call[0]) {
        'remote' { return $f.privateUrl }
        'ls-remote' {
            $remote = $call[1]
            $refName = $call[2]
            if ($remote -eq 'origin') {
                $target = if ($refName -eq $f.ref) { $f.privateTag } else { $f.newer }
            } elseif ($remote -eq $f.publicUrl) {
                $target = if ($refName -eq $f.ref) { $f.publicTag } else { $f.newer }
            } else { throw 'Unexpected remote in test' }
            if ($target) { return $target + [char]9 + $refName }
        }
        'merge-base' {
            if ($call[1] -ne '--is-ancestor' -or $call[2] -ne $f.head -or $call[3] -ne $f.newer) {
                throw 'Unexpected ancestry check in test'
            }
        }
        'push' {
            $f.pushes.Add(($call -join ' '))
            if ($call[1] -ne $f.publicUrl -or $call[2] -ne ($f.tag + ':' + $f.ref) -or $call.Count -ne 3) {
                throw 'Unexpected mutation in test'
            }
            $f.publicTag = $f.tag
        }
        default { throw "Unexpected git command in test: $($call[0])" }
    }
}

function Require-Rejection([string]$Action, [string]$Expected) {
    $result = ''
    try {
        & (Join-Path $PSScriptRoot 'publish_module.ps1') -Version $version -EvidenceDirectory $root -Action $Action -Python Test-Python
    } catch {
        $result = $_.Exception.Message
    }
    if ($result -notlike $Expected -or $pushes.Count) { throw "Expected rejection for $Action before any fake push; got: $result" }
}

try {
    Require-Rejection 'PrivateRefs' '*exact validated tag transaction*'
    @{commit=$head;tag_object=$tag;valid_until_utc='2025-01-01T00:00:00Z'} |
        ConvertTo-Json | Set-Content -LiteralPath $transaction -Encoding utf8
    Require-Rejection 'PrivateRefs' '*expired before the first outbound mutation*'
    $global:goOracleInfoPublishFixture.privateTag = $tag
    & (Join-Path $PSScriptRoot 'publish_module.ps1') -Version $version -EvidenceDirectory $root -Action PublicRefs -Python Test-Python
    if ($pushes.Count -ne 1 -or $global:goOracleInfoPublishFixture.publicTag -ne $tag) { throw 'Resume did not mirror the exact tag' }
    'Publisher transport passed: rejects missing and expired transactions, resumes exact tag without moving newer public main'
} finally {
    Remove-Variable -Scope Global -Name goOracleInfoPublishFixture
    if (Test-Path -LiteralPath $transaction) { Remove-Item -LiteralPath $transaction }
    if (@(Get-ChildItem -LiteralPath $root -Force).Count) { throw 'Unexpected test data; preserving the test directory' }
    Remove-Item -LiteralPath $root
}
