# Security

Please report suspected vulnerabilities privately through
[GitHub's security advisory form](https://github.com/sglavoie/muse/security/advisories/new).
If private reporting is unavailable, open an issue asking for a private contact
without including the vulnerability, exploit, or sensitive logs.

Include the affected version or commit, operating system, reproduction steps,
and expected impact. Remove personal file paths, music metadata, and credentials
from supporting material. Ordinary bugs belong in the public issue tracker.

Security fixes are developed on `main`. There is currently no commitment to
backport fixes to older releases or to respond within a particular time frame.

muse launches local tools (`mpv`, ExifTool, and/or ffprobe) to process media.
Keep those tools updated as well as muse. The local control sockets are intended
for processes running as your user; do not expose them over a network.
