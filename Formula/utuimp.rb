class Utuimp < Formula
  desc "Universal terminal user interface media player"
  homepage "https://github.com/tphx37/utuimp"
  license "GPL-3.0-or-later"
  head "https://github.com/tphx37/utuimp.git", branch: "main"

  depends_on "go" => :build
  depends_on "pkgconf" => :build
  depends_on "fontconfig"
  on_linux do
    depends_on "gstreamer"
  end

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/tphx37/utuimp/internal/version.Version=HEAD")
    prefix.install "NOTICE"
  end

  def caveats
    "For YouTube Music and SoundCloud: brew install yt-dlp ffmpeg\nFor Spotify Premium: brew install librespot"
  end
end
