# Source and public distribution

The private Gitea repository owns implementation history. The approved public
GitHub repository is github.com/marcuoli/go-oracle-info. Publish only reviewed,
integrated main and explicit immutable module tags, with identical commit IDs on
both forges. Public distribution includes the existing identity/memory packages
and the engineered-system facts. Private validation payloads remain outside Git.

Mirror pushes are additive and explicit: main plus the selected version tag.
Never force an update, delete refs, copy unrelated branches, or modify source on
the public copy. A conflicting branch/tag fails and requires investigation.
The initial delivery uses the existing authenticated publishing client; automatic
CI/deploy-key configuration is outside this release. Release pages are separate
on each forge. Verify remote visibility/refs, downloaded source and fresh public
module consumption before declaring publication complete.
