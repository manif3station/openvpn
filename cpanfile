requires 'perl', '5.38.0';

# Explicit core-module dependency record for this skill.
requires 'File::Basename';
requires 'File::Path';
requires 'File::Spec';
requires 'JSON::PP';
requires 'POSIX';
requires 'Digest::SHA';

# Skill-local modules used by the implementation:
# - OpenVPN::Launcher
# - OpenVPN::TOTP
