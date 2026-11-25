# Source and public distribution

The private Gitea repository owns implementation history. The public mirror is
`github.com/marcuoli/go-oracle-info`. Module publication uses a clean canonical
`main`, and the same integrated commit and immutable module tag on both forges.
Private validation inputs remain outside Git. The module embeds the four public
catalog snapshots by filename. Local Oracle Support captures, archives and
caller-supplied feeds belong in the private evidence store.

Use [the catalog refresh and review](release-catalog.md#refresh-before-publish)
for every new module version. Review the required scoped Oracle evidence using
available web sources and previously captured Oracle Support documents. Run
[`scripts/publish_module.ps1`](../scripts/publish_module.ps1) with PowerShell 7,
Git, Python, and the existing authenticated Tea and GitHub CLI clients. The
default `Check` action verifies the exact committed catalogs and writes a
version-, commit- and review-bound receipt before any mutation. Each version requires a fresh applicability review tied to the exact evidence bytes.

Use the same evidence directory and version for each authorized action in
order. Check and Tag require the current reviewed input paths. Tag validates
repository source and saves a separate transaction with the exact tag object
and catalog hashes. Subsequent actions require that saved transaction.

| Action | Result |
| --- | --- |
| `Check` | Verify the reviewed inputs, history and committed catalogs. |
| `Tag` | Run repository validation and create the exact local annotated tag. |
| `PrivateRefs` | Push and verify the tag on the already integrated private main. |
| `PublicRefs` | Push only the selected main and tag refs to the public mirror, atomically. |
| `PrivateRelease` | Create or verify the private release page with reviewed notes. |
| `PublicRelease` | Create or verify the public release page with reviewed notes. |

The first private tag push must occur before the version-bound applicability
review expires. After the exact private tag exists, resume remaining actions from
the saved transaction, even when the original review has expired. The
publisher verifies the exact tag object, committed catalogs, and containment
in private main. If public main already contains the release, it publishes
the missing tag without moving main. A divergent history or disagreeing
immutable ref stops the action. Never force or retarget either forge.

This path publishes refs and release notes without attaching custom archives.
Apply the project distribution rules separately to any authorized additional
asset. Verify remote visibility and fresh public module consumption before
declaring delivery complete.
