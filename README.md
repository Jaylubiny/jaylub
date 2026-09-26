## Jaylub

This repository is representing the source code behind website https://jaylub.com it is small website for starting company named jaylubiny.
Jaylubiny is not claimed as official compay yet and probably will not be for the next following 3-4 years couse of age restrictions of the employees.
We welcome any support or feedback on support@jaylub.com.


This readme specifically is for the creator of this website (preclik02) for notes of future updates or existing and yet comming routes or sub-domains of this webiste.
If you have any idea on update you can create issue on github or email us on support@jaylub.com (the response time should be within one day including weekends and holidays).







## ROUTES && SUB-DOMAINS

.com/pages/portfolio -> actually good portfolio

LATER ^

.com/updates

WORKING ON ^

/
/login
/logout
/me
/settings
/contacts
/docs
/documentation
/game/test
/game/jaylive
/game/jaylive/state
/game/jaylive/start
/game/jaylive/shop
/game/jaylive/character
/game/jaylive/run
/game/jaylive/leaderboard
/wiki
/wiki/C
/wiki/jaylub
/pages
/pages/jayware
/pages/jayware/download
/pages/jayos
/pages/nullc
/pages/allah
/pages/jaylub
/pages/services
/discord
/discord/allah/callback
/.well-known/discord
/chat
/chat/messages
/chat/send
/chat/files/<attachment-id>
/static/<file>
/web/static/<file>

DONE ^

## RUNNING UNDER SYSTEMD

`./run.sh` builds the server and runs it in the foreground so systemd can track
the process, collect its output, and stop it with SIGTERM. It does not create
`server.pid` or `server.log` files. Stop the configured unit with
`systemctl stop <unit>` or `./stop.sh <unit>`.

## UPDATES

add bioms and maps with stronger or weaker emeies (and in future custom enemies for every biom)

optimize the fuck out of this

bulvy jaylub that shoots 2 lazers

better chat experience and more secure chat

TowerTrack - run tracker for the game The Tower with parsed info into spreadsheet

bot sending message when an update is pushed to prod (on command by the owner - preclik02)
