# Castweb

This program provides the user a web GUI used to browse metadata representing videos for streaming.
The program takes an argument to a directory path, and on that path looks for a hierarchy of folders, having files with .strm and .nfo extensions. These files are Kodi compatible metadata files.

The .strm files contains just a single line, like 

        plugin://plugin.video.youtube/play/?video_id=zbKjqHqy2no

The relevant part is the last one video_id=zbKjqHqy2no, where zbKjqHqy2no specifies a Youtube video id

The .nfo files contain metadata about that video. Example

        <?xml version="1.0" encoding="UTF-8"?>
        <movie><title>Strange Filters</title><sorttitle>2025-06-11T14:23:20+00:00 Strange Filters</sorttitle><plot>Exploring a remarkably simple trick to create strange and colorful optical filter effects for useless but very cool visuals. By using a polarizer and something else... Don&#39;t forget to try the &#34;plastic filter&#34; yourself if you have a polarizer, or polarized sunglasses! 😎&#xA;&#xA;Music: https://posy.bandcamp.com/&#xA;Support my projects on Patreon: https://patreon.com/posy&#xA;&#xA;Lazy Posy: @lazyposy &#xA;&#xA;Notes: I have nothing against the steel factory. We need steel. It just suited the mood. &#34;That&#39;s so extreme!&#34; inspired by the &#39;Extreme Dudes&#39; in Harold &amp; Kumar Go to White Castle (2004)... I got the colorful pads in a local fairtrade shop.&#xA;&#xA;Ambiences and a Dance Track should appear on streaming at a later point.&#xA;Posy on Spotify: https://open.spotify.com/artist/3zkrmBzisZKngkrd6gXLHg&#xA;Or on Apple Music: https://music.apple.com/us/artist/posy/1522538629</plot><thumb>https://i3.ytimg.com/vi/zbKjqHqy2no/hqdefault.jpg</thumb><tag>Posy</tag></movie>

For each .strm and .nfo pair, sharing a common file name when ignoring the file extension, the program aggregates the metadata and presents it in a list in the web ui. It should allow the user to browse the files to see the metadata, and go up and down in the file hierarchy. 

There is a set of example data under the directory testdata. 

Running the server

- Build: `go build -o bin/castweb ./cmd/castweb`
- Run: `bin/castweb -root ./testdata -ytcast 12345678 -port 8080`
                                                      
When you click a video item or press Enter on it, the server executes:

    ytcast -d <ytcast-device-id> https://www.youtube.com/watch?v=<video_id>

Example:

    ytcast -d 12345678 'https://www.youtube.com/watch?v=6Td8dTnElAU'
