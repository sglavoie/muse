# Ghostty patches

`scripts/build-ghosttykit.sh` applies every `*.patch` here, in name order, to
the tag pinned in `../../GHOSTTY_VERSION` before building. Each patch exists
only because that tag does not yet carry the fix; re-check all of them when the
pin moves and delete the ones upstream absorbed.

- `0001-libtool-normalize-archives.patch` backports upstream's `ranlib`
  normalization in `src/build/LibtoolStep.zig` (on `main` after v1.3.1). Without
  it, Xcode 27's `libtool` drops every Zig-built member it deems misaligned, and
  `libghostty.a` ships without Ghostty's own objects.
- `0002-skip-libghostty-vt-install.patch` removes `libghostty-vt` from the default
  install step. Its shared library compiles Zig's bundled libc++, which fails
  against the macOS 27 SDK (`INFINITY` undeclared), and the app never uses it.
- `0003-modulemap-plain-header.patch` declares `ghostty.h` as a plain module
  header instead of an umbrella. `include/` also carries `libghostty-vt`'s
  headers, which `ghostty.h` never includes, so the umbrella form warns once per
  vt header on every Swift compile that imports `GhosttyKit`.
- `0004-dcimgui-unique-object-name.patch` renames dcimgui's `ext.cpp` to
  `dcimgui_ext.cpp`. glslang also compiles an `ext.cpp`, so `libghostty.a` held
  two `ext.o` members; `dsymutil` resolves `libghostty.a(ext.o)` to the first
  one, misses dcimgui's symbols, and warns while writing the Release dSYM.
