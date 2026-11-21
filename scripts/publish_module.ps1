[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$')][string]$Version,
    [Parameter(Mandatory)][string]$EvidenceDirectory,
    [ValidateSet('Check','Tag','PrivateRefs','PublicRefs','PrivateRelease','PublicRelease')][string]$Action = 'Check',
    [string]$Review,
    [string]$HarvesterDocs,
    [string]$OdaCaptures,
    [string]$ReleaseNotes,
    [string]$Python = 'python'
)
$ErrorActionPreference = 'Stop'
$private = 'https://git.lan.orcl.com.br/marcuoli/go-oracle-info.git'
$publicRepo = 'marcuoli/go-oracle-info'
$public = "https://github.com/$publicRepo.git"
$tagRef = "refs/tags/$Version"
$projectRoot = Split-Path -Parent $PSScriptRoot
$verifier = Join-Path $PSScriptRoot 'refresh_release_catalog.py'
if (-not [IO.Path]::IsPathFullyQualified($EvidenceDirectory)) { throw 'Use an absolute V: release evidence path' }
$evidencePath = [IO.Path]::GetFullPath($EvidenceDirectory)
$relative = [IO.Path]::GetRelativePath($projectRoot,$evidencePath)
if ($relative -eq '.' -or (-not $relative.StartsWith('..' + [IO.Path]::DirectorySeparatorChar) -and -not [IO.Path]::IsPathFullyQualified($relative))) {
    throw 'Keep the release evidence directory outside source control'
}
$transaction = Join-Path $evidencePath 'catalog-publish-transaction.json'

function Run-Git([string[]]$Arguments) {
    $result = & git @Arguments
    if ($LASTEXITCODE -ne 0) { throw "git $($Arguments[0]) failed ($LASTEXITCODE)" }
    return $result
}

function RemoteObject([string]$Remote, [string]$Ref) {
    $lines = @(Run-Git @('ls-remote',$Remote,$Ref))
    $selected = @($lines | Where-Object { $_.EndsWith([string][char]9 + $Ref) })
    if ($selected.Count -gt 1) { throw "More than one remote reference matched $Ref" }
    if ($selected.Count -eq 1) { return $selected[0].Split([char]9)[0] }
    return ''
}

function Is-Ancestor([string]$Older,[string]$Newer) {
    if (-not $Older -or -not $Newer) { return $false }
    & git merge-base --is-ancestor $Older $Newer
    if ($LASTEXITCODE -gt 1) { throw 'Remote history is unavailable locally; synchronize canonical main before retrying' }
    return $LASTEXITCODE -eq 0
}

function Check-NewCatalog {
    foreach ($path in @($Review,$HarvesterDocs,$OdaCaptures)) {
        if (-not $path -or -not [IO.Path]::IsPathFullyQualified($path)) {
            throw 'Check and Tag need absolute review, Harvester and ODA input paths'
        }
    }
    & $Python -B $verifier verify --version $Version --review $Review --harvester-docs $HarvesterDocs --oda-captures $OdaCaptures --output-dir $evidencePath
    if ($LASTEXITCODE -ne 0) { throw 'Catalog publish verification failed before any release mutation' }
}

function Check-Transaction {
    & $Python -B $verifier resume --version $Version --transaction $transaction
    if ($LASTEXITCODE -ne 0) { throw 'The exact validated tag transaction is required before any outbound release action' }
}

function Require-PrivateOrigin {
    if ((Run-Git @('remote','get-url','origin')) -ne $private -or (Run-Git @('remote','get-url','--push','origin')) -ne $private) {
        throw 'origin does not identify the approved private Gitea repository'
    }
}

function Save-Transaction($Receipt,[string]$Tag) {
    $Receipt | Add-Member -NotePropertyName tag_object -NotePropertyValue $Tag
    $json = $Receipt | ConvertTo-Json -Depth 6
    # A prior transaction is evidence for an immutable release. Never overwrite it.
    $stream = [IO.File]::Open($transaction,[IO.FileMode]::CreateNew)
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes($json + [Environment]::NewLine)
        $stream.Write($bytes,0,$bytes.Length)
    } finally {
        $stream.Dispose()
    }
}

function Read-PrivateRelease {
    $json = & tea releases ls --output json
    if ($LASTEXITCODE -ne 0) { throw 'Cannot verify existing private releases' }
    $rows = @($json | ConvertFrom-Json)
    if (@($rows | Where-Object { -not $_.PSObject.Properties['tag-_name'] }).Count) {
        throw 'Unrecognized Gitea release listing; cannot determine whether the release exists'
    }
    return $rows | Where-Object { $_.'tag-_name' -eq $Version }
}

Push-Location $projectRoot
try {
    if ($Action -in @('Check','Tag')) {
        Check-NewCatalog
        $receiptPath = Join-Path $evidencePath 'catalog-publish-receipt.json'
        $receipt = Get-Content -LiteralPath $receiptPath -Raw | ConvertFrom-Json
        $head = Run-Git @('rev-parse','HEAD')
        if ($receipt.commit -ne $head -or $receipt.version -ne $Version) { throw 'Catalog receipt does not match the checkout and tag' }
        if ($Action -eq 'Check') { return }
        Require-PrivateOrigin
        if ((RemoteObject 'origin' 'refs/heads/main') -ne $head) { throw 'Verify the actual private main before tagging' }
        if (Test-Path -LiteralPath $transaction) {
            Check-Transaction
            $saved = Get-Content -LiteralPath $transaction -Raw | ConvertFrom-Json
            if ($saved.commit -ne $head) { throw 'Existing release transaction points to a different commit' }
            return
        }
        & (Join-Path $PSScriptRoot 'validate.ps1') -Race -Scan
        Check-NewCatalog
        $receipt = Get-Content -LiteralPath $receiptPath -Raw | ConvertFrom-Json
        if ($receipt.commit -ne $head -or (RemoteObject 'origin' 'refs/heads/main') -ne $head) { throw 'Main changed during validation' }
        $present = @(Run-Git @('tag','--list',$Version)).Count -ne 0
        if (-not $present) { Run-Git @('tag','-a','-m',("Release " + $Version),$Version,$head) }
        $tag = Run-Git @('rev-parse','--verify',$tagRef)
        if ((Run-Git @('cat-file','-t',$tag)) -ne 'tag') { throw 'Expected an annotated module tag; existing lightweight tag preserved' }
        if ((Run-Git @('rev-parse',($tag + '^{commit}'))) -ne $head) { throw 'Existing local tag points elsewhere; immutable tag preserved' }
        $privateTag = RemoteObject 'origin' $tagRef
        if ($privateTag -and $privateTag -ne $tag) { throw 'Existing private immutable tag differs' }
        Save-Transaction $receipt $tag
        return
    }

    Check-Transaction
    Require-PrivateOrigin
    $receipt = Get-Content -LiteralPath $transaction -Raw | ConvertFrom-Json
    $head = $receipt.commit
    $tag = $receipt.tag_object
    $privateMain = RemoteObject 'origin' 'refs/heads/main'
    if (-not (Is-Ancestor $head $privateMain)) { throw 'The validated release commit is no longer contained in private main' }
    $privateTag = RemoteObject 'origin' $tagRef
    if ($privateTag -and $privateTag -ne $tag) { throw 'Private immutable tag differs from the validated object' }
    $tagPush = $tag + ':' + $tagRef
    if ($Action -eq 'PrivateRefs') {
        if (-not $privateTag) {
            if ([DateTimeOffset]::UtcNow -gt [DateTimeOffset]$receipt.valid_until_utc) {
                throw 'The review expired before the first outbound mutation; inspect the local tag and revalidate the release'
            }
            Run-Git @('push','origin',$tagPush)
        }
        if ((RemoteObject 'origin' $tagRef) -ne $tag) { throw 'Private tag verification failed' }
        return
    }
    if ($privateTag -ne $tag) { throw 'Publish and verify the checked private tag before further release actions' }
    if ($Action -in @('PublicRefs','PublicRelease')) {
        $mirrorTag = RemoteObject $public $tagRef
        $mirrorMain = RemoteObject $public 'refs/heads/main'
        if ($mirrorTag -and $mirrorTag -ne $tag) { throw 'Public immutable tag differs from the validated object' }
        if ($Action -eq 'PublicRefs') {
            if (-not (Is-Ancestor $head $mirrorMain)) {
                if ($mirrorMain -and -not (Is-Ancestor $mirrorMain $head)) { throw 'Public main diverges from the verified release' }
                Run-Git @('push','--atomic',$public,($head + ':refs/heads/main'),$tagPush)
            } elseif ($mirrorTag -ne $tag) {
                # An intervening public release must never be rolled back.
                Run-Git @('push',$public,$tagPush)
            }
            if ((RemoteObject $public $tagRef) -ne $tag -or -not (Is-Ancestor $head (RemoteObject $public 'refs/heads/main'))) {
                throw 'Public main or tag verification failed'
            }
            return
        }
        if ($mirrorTag -ne $tag -or -not (Is-Ancestor $head $mirrorMain)) {
            throw 'Use the verified public refs before creating the public release page'
        }
    }

    if (-not $ReleaseNotes -or -not [IO.Path]::IsPathFullyQualified($ReleaseNotes) -or -not (Test-Path -LiteralPath $ReleaseNotes -PathType Leaf)) {
        throw 'An absolute, reviewed release notes file is required'
    }
    if ($Action -eq 'PrivateRelease') {
        $matched = @(Read-PrivateRelease)
        if ($matched.Count -eq 0) {
            & tea releases create --tag $Version --target $head --title $Version --note-file $ReleaseNotes
            if ($LASTEXITCODE -ne 0) { throw 'Private release creation failed; reread live state before retrying' }
            $matched = @(Read-PrivateRelease)
        }
        if ($matched.Count -ne 1 -or $matched[0].status -ne 'released') {
            throw 'Private release verification failed'
        }
    } elseif ($Action -eq 'PublicRelease') {
        $existing = & gh release view $Version --repo $publicRepo --json tagName,isDraft,isPrerelease 2>&1
        if ($LASTEXITCODE -ne 0) {
            if (($existing | Out-String) -notmatch '(?im)^\s*release not found\s*$') { throw 'Cannot verify existing public release' }
            & gh release create $Version --verify-tag --repo $publicRepo --title $Version --notes-file $ReleaseNotes
            if ($LASTEXITCODE -ne 0) { throw 'Public release creation failed; reread live state before retrying' }
            $existing = & gh release view $Version --repo $publicRepo --json tagName,isDraft,isPrerelease
            if ($LASTEXITCODE -ne 0) { throw 'Cannot verify the public release' }
        }
        $published = $existing | ConvertFrom-Json
        if ($published.tagName -ne $Version -or $published.isDraft -or $published.isPrerelease) {
            throw 'Public release verification failed'
        }
    }
} finally {
    Pop-Location
}
