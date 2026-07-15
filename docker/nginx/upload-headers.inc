# Headers for user-uploaded files, shared by the disk and bucket paths.
#
# The same set the API sends when it serves /uploads itself — moving the files
# behind a CDN must not quietly drop them.
#
# `always` matters: without it these are attached only to 2xx/3xx, so an error
# response would carry none of them.

# The upload service identifies files by their bytes and stores them under a
# matching extension. nosniff stops a browser overriding that and deciding some
# image is HTML, which from a origin serving user content is a script.
add_header X-Content-Type-Options "nosniff" always;

# Nothing here should load anything, and nothing should frame it.
add_header Content-Security-Policy "default-src 'none'; sandbox" always;

# These are images meant to render in a page, not downloads.
add_header Content-Disposition "inline" always;
