use strict;
use warnings;
use Test::More;
use File::Spec;

my $license = File::Spec->catfile( '.', 'LICENSE' );
my $readme  = File::Spec->catfile( '.', 'README.md' );

ok( -f $license, 'LICENSE exists' );
ok( -f $readme,  'README exists' );

open my $license_fh, '<', $license or die "Unable to read $license: $!";
my $license_text = do { local $/; <$license_fh> };
close $license_fh;

like( $license_text, qr/^MIT License$/m, 'LICENSE declares MIT heading' );
like(
    $license_text,
    qr/Permission is hereby granted, free of charge, to any person obtaining a copy/,
    'LICENSE includes MIT grant text',
);
like(
    $license_text,
    qr/THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\s+IMPLIED,/s,
    'LICENSE includes MIT disclaimer',
);

open my $readme_fh, '<', $readme or die "Unable to read $readme: $!";
my $readme_text = do { local $/; <$readme_fh> };
close $readme_fh;

like( $readme_text, qr/^## License$/m, 'README includes license section' );
like( $readme_text, qr/`openvpn` is released under the MIT License\./, 'README mentions MIT license' );
like( $readme_text, qr/See \[LICENSE\]\(LICENSE\)\./, 'README links to LICENSE file' );

done_testing;
