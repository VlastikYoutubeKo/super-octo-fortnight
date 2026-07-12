import sys

prompt = """
PL-[EXTRA]: POLSAT SPORT [PLUS] PPV4 [1080p]	
Polsat.Sport.Premium.5.pl

PL-[EXTRA]: POLSAT SPORT [PLUS] PPV6 [1080p]	
Polsat.Sport.Premium.5.pl

PL-[EXTRA]: 13 ULICA [1080p]	
13.Ulica.HD.pl

PL-[EXTRA]: 4 FUN DANCE [1080p]	
Polsat.Sport.Premium.4.pl

PL-[EXTRA]: 4 FUN KIDS [1080p]	
Polsat.Sport.Premium.4.pl

PL-[EXTRA]: 4 FUN TV [1080p]	
TV.4.HD.pl

PL-[EXTRA]: AlLE INO+ [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: AMC POLSKA [1080p]	
Kino.Polska.Muzyka.pl

PL-[EXTRA]: ANIMAL PLANET [1080p]	
ANIMAL.PLANET.HD.(Animal.Planet.HD).pe

PL-[EXTRA]: AXN [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: AXN BLACK [1080p]	
AXN.Black.pl

PL-[EXTRA]: AXN WHITE [1080p]	
AXN.White.pl

PL-[EXTRA]: BBC BRIT [1080p]	
BBC.Brit.HD.pl

PL-[EXTRA]: BBC CBEEBIES POLSKA [1080p]	
BBC.CBeebies.pl

PL-[EXTRA]: BBC FIRST [1080p]	
BBC.First.HD.pl

PL-[EXTRA]: BBC LIFESTYLE POLSKA [1080p]	
BBC.Lifestyle.HD.pl

PL-[EXTRA]: BOOMERANG [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: CANAL+ DOKUMENT [1080p]	
CANAL+.DOKUMENT.HD.pl

PL-[EXTRA]: CANAL+ DOMO [1080p]	
CANAL+.DOMO.HD.pl

PL-[EXTRA]: CANAL+ FAMILY [1080p]	
CANAL+.Family.HD.pl

PL-[EXTRA]: CANAL+ KUCHNIA [1080p]	
CANAL+.KUCHNIA.HD.pl

PL-[EXTRA]: CANAL+ NOW [1080p]	
CANAL+.EXTRA.13.HD.pl

PL-[EXTRA]: CANAL+ SPORT 5 POLSKA [1080p]	
CANAL+.Sport.5.HD.pl

PL-[EXTRA]: CARTOON NETWORK [1080p]	
Cartoon.Network.HD.(Cartoon.Network.HD).pe

PL-[EXTRA]: CBS EUROPA [1080p]	
CBS.Europa.HD.pl

PL-[EXTRA]: CBS RRALITY [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: CI POLSAT [1080p]	
CI.Polsat.HD.pl

PL-[EXTRA]: CINEMAX [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: CINEMAX 2 [1080p]	
France.2.-.PL.pl

PL-[EXTRA]: DA VINCI POLSKA [1080p]	
Da.Vinci.HD.pl

PL-[EXTRA]: DISCO POLO MUSIC [1080p]	
Disco.Polo.Music.pl

PL-[EXTRA]: DISCOVERY [1080p]	
Discovery.Channel.(niem.).pl

PL-[EXTRA]: DISCOVERY HISTORIA [1080p]	
Discovery.Historia.pl

PL-[EXTRA]: DISNEY CHANNEL [1080p]	
Disney.Channel.HD.pl

PL-[EXTRA]: DISNEY JUNIOR [1080p]	
Disney.Junior.pl

PL-[EXTRA]: DISNEY XD [1080p]	
Disney.XD.pl

PL-[EXTRA]: E ENTERTAINMENT [1080p]	
E!.Entertainment.HD.pl

PL-[EXTRA]: ELEVEN SPORTS 1 [1080p]	
Eleven.Sports.1.HD.pl

PL-[EXTRA]: ELEVEN SPORTS 2 [1080p]	
Eleven.Sports.2.HD.pl

PL-[EXTRA]: ELEVEN SPORTS 3 [1080p]	
Eleven.Sports.3.HD.pl

PL-[EXTRA]: EPIC DRAMA [1080p]	
Epic.Drama.HD.pl

PL-[EXTRA]: ESKA ROCK TV [1080p]	
Eska.Rock.TV.pl

PL-[EXTRA]: ESKA TV [1080p]	
Eska.TV.Extra.HD.pl

PL-[EXTRA]: EUROSPORT 1 [1080p]	
Eurosport.1.(niem.).pl

PL-[EXTRA]: EUROSPORT 2 [1080p]	
Eurosport.2.HD.pl

PL-[EXTRA]: EXTREME SPORTS [1080p]	
[EXTRSP].Extreme.Sports.Channel.se

PL-[EXTRA]: FILMBOX ARTHOUSE [1080p]	
FilmBox.Arthouse.HD.pl

PL-[EXTRA]: FILMBOX EXTRA [1080p]	
FilmBox.Extra.HD.pl

PL-[EXTRA]: FILMBOX FAMILY [1080p]	
FilmBox.Family.pl

PL-[EXTRA]: FOKUS TV [1080p]	
Fokus.TV.HD.pl

PL-[EXTRA]: FOOD NETWORK [1080p]	
Food.Network.HD.-.EN.pl

PL-[EXTRA]: FOX [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: FOX COMEDY POLSKA [1080p]	
FOX.Comedy.HD.pl

PL-[EXTRA]: FX [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: FX COMEDY [1080p]	
Polsat.Comedy.Central.Extra.pl

PL-[EXTRA]: GOLF CHANNEL [1080p]	
Golf.Channel.HD.pl

PL-[EXTRA]: HBO [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: HBO 2 [1080p]	
HBO.2.HD.(HBO.2.HD).pe

PL-[EXTRA]: HBO 3 [1080p]	
Polsat.Sport.Premium.3.pl

PL-[EXTRA]: HISTORY [1080p]	
Polsat.Viasat.History.HD.pl

PL-[EXTRA]: HISTORY 2 POLSKA [1080p]	
France.2.-.PL.pl

PL-[EXTRA]: INVESTIGATION DISCOVERY POLSKA [1080p]	
INVESTIGATION.DISCOVERY.(Invest..Discovery).pe

PL-[EXTRA]: MINIMINI+ [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: MTV LIVE [1080p]	
MTV.Live.HD.pl

PL-[EXTRA]: MTV POLSKA [1080p]	
MTV.Polska.HD.pl

PL-[EXTRA]: NATIONAL GEO [1080p]	
National.Geographic.Wild.HD.pl

PL-[EXTRA]: NATIONAL GEOGRAPHIC WILD [1080p]	
National.Geographic.Wild.HD.pl

PL-[EXTRA]: NICK JR [1080p]	
Nick.Jr..HD.pl

PL-[EXTRA]: NICKELODEON [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: NOVELA TV [1080p]	
Novela.tv.HD.pl

PL-[EXTRA]: NOWA TV [1080p]	
Nowa.TV.HD.pl

PL-[EXTRA]: PARAMOUNT CHANNEL POLSKA [1080p]	
Canal.Paramount.Channel.Latinoamérica.sv

PL-[EXTRA]: PLANETE+ [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: POLO TV [1080p]	
Polo.TV.HD.pl

PL-[EXTRA]: POLSAT [1080p]	
Polsat.Viasat.Explore.HD.pl

PL-[EXTRA]: POLSAT CAFE [1080p]	
Polsat.Comedy.Central.Extra.HD.pl

PL-[EXTRA]: POLSAT DOKU [1080p]	
Polsat.Doku.HD.pl

PL-[EXTRA]: POLSAT FILM [1080p]	
Polsat.Film.HD.pl

PL-[EXTRA]: POLSAT GAMES [1080p]	
Polsat.Games.HD.pl

PL-[EXTRA]: POLSAT JIM JAM [1080p]	
Polsat.JimJam.pl

PL-[EXTRA]: POLSAT MUSIC [1080p]	
Polsat.Music.HD.pl

PL-[EXTRA]: POLSAT NEWS [1080p]	
Polsat.Sport.News.HD.pl

PL-[EXTRA]: POLSAT NEWS POLITYKA [1080p]	
Polsat.Sport.News.HD.pl

PL-[EXTRA]: POLSAT PLAY [1080p]	
Polsat.Play.HD.pl

PL-[EXTRA]: POLSAT RODZINA [1080p]	
Polsat.Rodzina.HD.pl

PL-[EXTRA]: POLSAT SERIALE [1080p]	
Polsat.Seriale.HD.pl

PL-[EXTRA]: POLSAT SPORT [PLUS] PPV3 [1080p]	
Polsat.Sport.Premium.5.pl

PL-[EXTRA]: POLSAT SPORT [PLUS] PPV5 [1080p]	
Polsat.Sport.Premium.5.pl

PL-[EXTRA]: POLSAT SPORT 1 [1080p]	
Polsat.Sport.Premium.1.pl

PL-[EXTRA]: POLSAT SPORT 2 [1080p]	
Polsat.Sport.Premium.2.pl

PL-[EXTRA]: POLSAT SPORT 3 [1080p]	
Polsat.Sport.Premium.3.pl

PL-[EXTRA]: POLSAT SPORT FIGHT [1080p]	
Polsat.Sport.Fight.HD.pl

PL-[EXTRA]: POLSAT VIASAT HISTORY [1080p]	
Polsat.Viasat.History.HD.pl

PL-[EXTRA]: PULS 2 [1080p]	
France.2.-.PL.pl

PL-[EXTRA]: ROMANCE TV [1080p]	
Romance.TV.HD.pl

PL-[EXTRA]: SCIENCE POLAND [1080p]	
Discovery.Science.HD.pl

PL-[EXTRA]: SCIFI POLSKA [1080p]	
Kino.Polska.Muzyka.pl

PL-[EXTRA]: SKY SHOWTIME 1 [1080p]	
Sky.Sport.HD.1.pl

PL-[EXTRA]: SKY SHOWTIME 2 [1080p]	
France.2.-.PL.pl

PL-[EXTRA]: STOPKLATKA [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TEENNICK POLSKA [1080p]	
Kino.Polska.Muzyka.pl

PL-[EXTRA]: TELETOON+ [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TLC [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TRAVEL CHANNEL [1080p]	
Travel.Channel.HD.pl

PL-[EXTRA]: TTV [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TV PULS [1080p]	
TV.Puls.HD.pl

PL-[EXTRA]: TV REPUBLIKA [1080p]	
TV.Republika.HD.pl

PL-[EXTRA]: TV4 [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TV6 [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TVN [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TVN 24 [1080p]	
TVN.24.HD.pl

PL-[EXTRA]: TVN 7 [1080p]	
TVN.7.HD.pl

PL-[EXTRA]: TVN FABULA [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: TVN STYLE [1080p]	
TVN.Style.HD.pl

PL-[EXTRA]: TVN TURBO [1080p]	
TVN.Turbo.HD.pl

PL-[EXTRA]: TVN24 BIS [1080p]	
TVN24.BiS.HD.pl

PL-[EXTRA]: TVP [1080p]	
TVP.3.Białystok.pl

PL-[EXTRA]: TVP 1 [1080p]	
TVP.1.HD.pl

PL-[EXTRA]: TVP 2 [1080p]	
France.2.-.PL.pl

PL-[EXTRA]: TVP ABC [1080p]	
TVP.ABC.pl

PL-[EXTRA]: TVP HISTORIA [1080p]	
TVP.Historia.pl

PL-[EXTRA]: TVP KULTURA [1080p]	
TVP.Kultura.HD.pl

PL-[EXTRA]: TVP SERIALE [1080p]	
TVP.Seriale.pl

PL-[EXTRA]: TVP SPORT [1080p]	
TVP.Sport.HD.pl

PL-[EXTRA]: TVS [1080p]	
tvregionalna.pl.pl

PL-[EXTRA]: VIASAT EXPLORE POLSAT [1080p]	
Polsat.Viasat.Explore.HD.pl

PL-[EXTRA]: VIASAT NATURE POLSAT [1080p]	
Polsat.Viasat.Nature.HD.pl

PL-[EXTRA]: VOX MUSIC TV [1080p]	
VOX.Music.TV.pl

PL-[EXTRA]: WARNER TV POLSKA [1080p]	
Warner.TV.HD.pl

PL-[EXTRA]: WPOLSCE.PL [1080p]	
wPolsce.pl.HD.pl

PL-[EXTRA]: ZOOM TV [1080p]	
ZOOM.TV.HD.pl

PL-[EXTRA]TVP 3 [1080p]	
ESPN.3.HD.(ESPN.3.HD).pe

PL: CHILLI ZET [1080p]	
tvregionalna.pl.pl

PL: RADIO POGODA [1080p]	
Polskie.Radio.Program.2.pl

PL: RADIO ZET [1080p]	
Radio.Nowy.Świat.pl

PL: RMF MAXXX [1080p]	
tvregionalna.pl.pl

PL: SMOOTH JAZZ [1080p]	
plex.tv.Smooth.Jazz.plex

PL: TOK FM [1080p]	
tvregionalna.pl.pl

PL: [480p] TUNEBOX [720p]	
tvregionalna.pl.pl

PL: 4FUN DANCE [1080p]	
4FUN.DANCE.pl

PL: 4FUN GOLD HITS [1080p]	
tvregionalna.pl.pl

PL: 4FUN KIDS [1080p]	
4FUN.KIDS.pl

PL: 4FUN.TV [1080p]	
4FUN.TV.pl

PL: 500 R'N'B HITS [1080p]	
tvregionalna.pl.pl

PL: 90S HITS [1080p]	
tvregionalna.pl.pl

PL: ACTIVE FAMILY [1080p]	
Active.Family.HD.pl

PL: ADVENTURE [1080p]	
tvregionalna.pl.pl

PL: ALE KINO+ [1080p]	
Ale.kino+.HD.pl

PL: ALT CLASSIC [1080p]	
tvregionalna.pl.pl

PL: AMC [1080p]	
wPolsce.pl.HD.pl

PL: ANTENA TV [1080p]	
plex.tv.DREAD.TV.plex

PL: ARKA NOEGO KOLĘDY PIOSENKI ŚWIĄTECZNE POL [1080p]	
tvregionalna.pl.pl

PL: AXN SPIN [1080p]	
AXN.Spin.HD.pl

PL: BABY TV [1080p]	
plex.tv.BABY.SHARK.TV.plex

PL: BBC CBEEBIES [1080p]	
BBC.CBeebies.pl

PL: BBC EARTH [1080p]	
BBC.Earth.HD.pl

PL: BBC LIFESTYLE [1080p]	
BBC.Lifestyle.HD.pl

PL: BIZNES24 [1080p]	
tvregionalna.pl.pl

PL: CANAL+ [PLUS] [720p] (NA)	
CANAL+.EXTRA.17.HD.pl

PL: CANAL+ 1 [720p] (NA)	
CANAL+.EXTRA.1.HD.pl

PL: CANAL+ FILM [720p] (NA)	
CANAL+.Film.HD.pl

PL: CANAL+ SERIALE [720p] (NA)	
CANAL+.Seriale.HD.pl

PL: CANAL+ SPORT [1080p]	
CANAL+.Sport.5.HD.pl

PL: CANAL+ SPORT 2 [1080p]	
CANAL+.Sport.2.HD.pl

PL: CANAL+ SPORT 3 [1080p]	
CANAL+.Sport.3.HD.pl

PL: CANAL+ SPORT 4 [1080p]	
CANAL+.Sport.4.HD.pl

PL: CANAL+ SPORT 5 [1080p]	
CANAL+.Sport.5.HD.pl

PL: CBS REALITY [1080p]	
CBS.Reality.pl

PL: CLASSIC METAL [1080p]	
tvregionalna.pl.pl

PL: CLOUT MMA [1080p]	
tvregionalna.pl.pl

PL: COMEDY CENTRAL [1080p]	
Polsat.Comedy.Central.Extra.pl

PL: COMEDY CENTRAL POLAND [1080p]	
Polsat.Comedy.Central.Extra.HD.pl

PL: CREMA CAFÉ [1080p]	
Polsat.Café.HD.pl

PL: CRIME AND INVESTIGATION [1080p]	
Crime.and.Investigation.Network.HD.us2

PL: DA VINCI LEARNING [1080p]	
DA.VINCI.LEARNING.HD.tr

PL: DISCO POLO [1080p]	
Disco.Polo.Music.pl

PL: DISCOVERY LIFE [1080p]	
Discovery.Life.HD.pl

PL: DISCOVERY SCIENCE [1080p]	
Discovery.Science.HD.pl

PL: DOMO+ [720p] (NA)	
tvregionalna.pl.pl

PL: DTX [1080p]	
wPolsce.pl.HD.pl

PL: DUCK TV [720p]	
TV.Republika.HD.pl

PL: DUCK TV PLUS [720p]	
ducktv.plus.pl

PL: E! ENTERTAINMENT [1080p]	
E!.Entertainment.HD.pl

PL: ELEVEN SPORTS 1 [4K-Q]/[4K-Q]	
Eleven.Sports.1.4K.pl

PL: ELEVEN SPORTS 1 [4K-Q]+	
Eleven.Sports.1.HD.pl

PL: ELEVEN SPORTS 4 [1080p]	
Eleven.Sports.4.HD.pl

PL: ENEJ KOLĘDY POD WSPÓLNYM NIEBEM [1080p]	
tvregionalna.pl.pl

PL: ESKA TV EXTRA [1080p]	
Eska.TV.Extra.HD.pl

PL: EUROSPORT [4K-Q]	
tvregionalna.pl.pl

PL: EUROSPORT 3 [1080p]	
ESPN.3.HD.(ESPN.3.HD).pe

PL: EUROSPORT 4 [1080p]	
Eurosport.1.(niem.).pl

PL: EUROSPORT 5 [1080p]	
Eurosport.1.(niem.).pl

PL: EUROSPORT 6 [1080p]	
Eurosport.1.(niem.).pl

PL: EUROSPORT 7 [1080p]	
Eurosport.1.(niem.).pl

PL: EUROSPORT 8 [1080p]	
Eurosport.1.(niem.).pl

PL: EUROSPORT 9 [1080p]	
Eurosport.1.(niem.).pl

PL: EWTN POLSKA [1080p]	
Kino.Polska.Muzyka.pl

PL: FAME MMA [1080p]	
tvregionalna.pl.pl

PL: FEN MMA [1080p]	
tvregionalna.pl.pl

PL: FIGHTBOX [1080p]	
tvregionalna.pl.pl

PL: FIGHTKLUB [1080p]	
tvregionalna.pl.pl

PL: FILMBOX [PLUS] [1080p]	
FilmBox.Arthouse.HD.pl

PL: FILMBOX ACTION [1080p]	
FilmBox.Action.pl

PL: FOX COMEDY [1080p]	
FOX.Comedy.HD.pl

PL: GROMDA [1080p]	
tvregionalna.pl.pl

PL: GRONICKI NAJPIĘKNIEJSZE KOLĘDY POLSKIE I GÓRALSKIE [1080p]	
Czwórka.Polskie.Radio.HD.pl

PL: H2 [1080p]	
wPolsce.pl.pl

PL: HGTV [1080p]	
wPolsce.pl.HD.pl

PL: HIGH LEAGUE [1080p]	
tvregionalna.pl.pl

PL: HIGH LEAGUE 6 [1080p]	
Polsat.Sport.Premium.6.pl

PL: HIGH LEAGUE 6 BACKUP 1 [1080p]	
Now.Sports.Premier.League.6.hk

PL: HIGH LEAGUE 6 BACKUP 2 [1080p]	
France.2.-.PL.pl

PL: HOME TV [1080p]	
HOME.TV.HD.pl

PL: CHANNEL 13 (NA) [1080p]	
Discovery.Channel.(niem.).pl

PL: ID [1080p]	
wPolsce.pl.pl

PL: ITALO DISCO [1080p]	
Disco.Polo.Music.pl

PL: ITVN [720p] (NA)	
tvregionalna.pl.pl

PL: ITVN EXTRA (NA) [1080p]	
iTVN.Extra.International.pl

PL: JAZZ [720p]	
wPolsce.pl.HD.pl

PL: KAPELA GÓROLE KOLĘDY GÓRALSKIE [1080p]	
tvregionalna.pl.pl

PL: KINO POLSKA [720p] (NA)	
Kino.Polska.Muzyka.pl

PL: KINO POLSKA MUZYKA [1080p]	
Kino.Polska.Muzyka.pl

PL: KINO TV [1080p]	
Kino.TV.HD.pl

PL: KOLĘDY I PASTORAŁKI GÓRALSKIE KAPELA OGÓRKI [1080p]	
tvregionalna.pl.pl

PL: KOLEDY I PASTORALKI GÓRALSKIE SPOD SAMIUCKIK TATER [1080p]	
tvregionalna.pl.pl

PL: KOLĘDY Z ZENKIEM MARTYNIUKIEM [1080p]	
tvregionalna.pl.pl

PL: KOMINEK I MUZYKA RELAKSACYJNO BOŻONARODZENIOWA 1 [1080p]	
Polskie.Radio.Program.1.pl

PL: KOMINEK I MUZYKA RELAKSACYJNO BOŻONARODZENIOWA 2 [1080p]	
France.2.-.PL.pl

PL: KOMINEK I ZAGRANICZNE PIOSENKI ŚWIĄTECZNE 1 [1080p]	
Polskie.Radio.Program.1.pl

PL: KOMINEK I ZAGRANICZNE PIOSENKI ŚWIĄTECZNE 2 [1080p]	
France.2.-.PL.pl

PL: KOMINEK ŚWIĄTECZNY 1 [1080p]	
Polskie.Radio.Program.1.pl

PL: KOMINEK ŚWIĄTECZNY 2 [1080p]	
France.2.-.PL.pl

PL: KONCERT KOLĘD I PASTORAŁEK MAŁEJ ARMII JANOSIKA W ROKICINACH PODHALAŃSKICH [1080p]	
tvregionalna.pl.pl

PL: KSW [1080p]	
wPolsce.pl.HD.pl

PL: KUCHNIA+ [720p] (NA)	
tvregionalna.pl.pl

PL: LOVE NATURE [4K-Q]+	
Love.Nature.4K.pl

PL: LUBELSKA TV (NA) [1080p]	
Lubelska.tv.HD.pl

PL: MATTEO BOCELLI I PRZYJACIELE ŚWIĘTA SPEŁNIONYCH MARZEŃ [1080p]	
tvregionalna.pl.pl

PL: MAZOWSZE ŚPIEWA KOLĘDY [1080p]	
tvregionalna.pl.pl

PL: METRO [1080p]	
wPolsce.pl.HD.pl

PL: MEZZO [720p]	
France.2.-.PL.pl

PL: MEZZO LIVE [720p]	
Mezzo.Live.HD.pl

PL: MINIMINI [1080p]	
tvregionalna.pl.pl

PL: MIXTAPE [720p]	
tvregionalna.pl.pl

PL: MOTOWIZJA [1080p]	
tvregionalna.pl.pl

PL: MTV 00S [1080p]	
MTV.00s.pl

PL: MTV 80S [720p]	
MTV.80s.pl

PL: MUSIC BOX POLSKA [1080p]	
MUSIC.BOX.POLSKA.musicbox

PL: NAJLEPSZA RELAKSACYJNO ZAGRANICZNA MUZYKA ŚWIĄTECZNA PRZY KOMINKU [1080p]	
Kino.Polska.Muzyka.pl

PL: NAJLEPSZE ZAGRANICZNE PIOSENKI ŚWIĄTECZNE PRZY CHOINCE [1080p]	
tvregionalna.pl.pl

PL: NAJLEPSZE ZAGRANICZNE PIOSENKI ŚWIĄTECZNE Z TEKSTEM [1080p]	
tvregionalna.pl.pl

PL: NAJLEPSZE ZAGRANICZNE PIOSENKI ŚWIĄTECZNE ZIMA [1080p]	
tvregionalna.pl.pl

PL: NAJPIĘKNIEJSZE KOLĘDY I PASTORAŁKI GÓRALSKIE [1080p]	
tvregionalna.pl.pl

PL: NAJPIĘKNIEJSZE KOLĘDY KRZYSZTOF KRAWCZYK [1080p]	
tvregionalna.pl.pl

PL: NAJPIĘKNIEJSZE KOLĘDY Z PODHALA [1080p]	
tvregionalna.pl.pl

PL: NAJPIĘKNIEJSZE POLSKIE KOLĘDY DLA DZIECI [1080p]	
Czwórka.Polskie.Radio.HD.pl

PL: NAJPIĘKNIEJSZE POLSKIE KOLĘDY GOLEC UORKIESTRA [1080p]	
Czwórka.Polskie.Radio.HD.pl

PL: NAJPIĘKNIEJSZE POLSKIE KOLĘDY GOLEC UORKIESTRA 3 JASNA GÓRA [1080p]	
TVP.3.Gorzów.Wielkopolski.pl

PL: NAJPIĘKNIEJSZE POLSKIE KOLĘDY Z TEKSTEM [1080p]	
Czwórka.Polskie.Radio.HD.pl

PL: NAT GEO PEOPLE [1080p]	
Nat.Geo.People.HD.pl

PL: NAT GEO WILD [1080p]	
Nat.Geo.Wild.HD.(BIH).ba

PL: NATIONAL GEOGRAPHIC [1080p]	
National.Geographic.Wild.HD.pl

PL: NICK JR POLSKA [1080p]	
Nick.Jr..HD.pl

PL: NICK MUSIC [720p]	
Nick.Music.(BIH).ba

PL: NICKTOONS [1080p]	
tvregionalna.pl.pl

PL: NOVELAS+ [1080p]	
tvregionalna.pl.pl

PL: NUTA TV [1080p]	
Nuta.TV.HD.pl

PL: OKTAGON MMA [1080p]	
tvregionalna.pl.pl

PL: PARAMOUNT CHANNEL [1080p]	
Discovery.Channel.(niem.).pl

PL: PIOSENKI ŚWIĄTECZNE DLA DZIECI [1080p]	
tvregionalna.pl.pl

PL: POLSAT 1 (NA) [1080p]	
Polsat.Sport.Premium.1.pl

PL: POLSAT 2 [1080p]	
Polsat.News.2.HD.pl

PL: POLSAT COMEDY CENTRAL EXTRA [1080p]	
Polsat.Comedy.Central.Extra.HD.pl

PL: POLSAT NEWS 2 [1080p]	
Polsat.News.2.HD.pl

PL: POLSAT SPORT [PLUS] 4 [1080p]	
Polsat.Sport.Premium.4.pl

PL: POLSAT SPORT [PLUS] 5 [1080p]	
Polsat.Sport.Premium.5.pl

PL: POLSAT SPORT [PLUS] 6 [1080p]	
Polsat.Sport.Premium.6.pl

PL: POLSAT SPORT EXTRA 1 [1080p]	
Polsat.Sport.Premium.1.pl

PL: POLSAT SPORT EXTRA 2 [1080p]	
Polsat.Sport.Premium.2.pl

PL: POLSAT SPORT EXTRA 3 [1080p]	
Polsat.Sport.Premium.3.pl

PL: POLSAT SPORT EXTRA 4 [1080p]	
Polsat.Sport.Premium.4.pl

PL: POLSAT VIASAT EXPLORE [1080p]	
Polsat.Viasat.Explore.HD.pl

PL: POLSAT VIASAT NATURE [1080p]	
Polsat.Viasat.Nature.HD.pl

PL: POLSAT X [1080p]	
Polsat.Rodzina.HD.pl

PL: POLSAT2 [720p] (NA)	
tvregionalna.pl.pl

PL: POWER TV [1080p]	
Power.TV.HD.pl

PL: PRIME FIGHT [720p]	
Prime.Fight.HD.pl

PL: PRIME MMA [1080p]	
tvregionalna.pl.pl

PL: PULS [1080p]	
France.2.-.PL.pl

PL: RADIOPARTY KANAŁ GŁÓWNY [1080p]	
tvregionalna.pl.pl

PL: RED CARPET TV [1080p]	
Red.Carpet.TV.HD.pl

PL: RMF 80S [1080p]	
tvregionalna.pl.pl

PL: RMF BLUES [1080p]	
tvregionalna.pl.pl

PL: RMF DEPECHE MODE [1080p]	
tvregionalna.pl.pl

PL: SCIFI UNIVERSAL [1080p]	
tvregionalna.pl.pl

PL: SPORTKLUB [1080p]	
tvregionalna.pl.pl

PL: STARS.TV [1080p]	
STARS.TV.HD.pl

PL: STOPKLATKA TV [1080p]	
plex.tv.Desi.Play.TV.plex

PL: STUDIOMED TV [1080p]	
StudioMED.TV.pl

PL: SUNDANCE TV [1080p]	
Sundance.TV.HD.pl

PL: SUPER POLSAT [1080p]	
Super.Polsat.HD.pl

PL: ŚWIĄTECZNE ZAGRANICZNE PIOSENKI DLA DZIECI [1080p]	
tvregionalna.pl.pl

PL: ŚWIĄTECZNY KOMINEK [1080p]	
tvregionalna.pl.pl

PL: ŚWIĘTA NA KLUBOWO [1080p]	
tvregionalna.pl.pl

PL: ŚWIĘTA Z BONEY M PRZY KOMINKU [1080p]	
tvregionalna.pl.pl

PL: TBN POLSKA [1080p]	
TBN.Polska.HD.pl

PL: TEENNICK [1080p]	
tvregionalna.pl.pl

PL: TELE 5 [1080p]	
Tele.5.(niem.).pl

PL: THE WAR [1080p]	
tvregionalna.pl.pl

PL: TOP KIDS [1080p]	
Top.Kids.HD.pl

PL: TRANCE [1080p]	
tvregionalna.pl.pl

PL: TRWAM TV [1080p]	
TV.Trwam.pl

PL: TVN CZAS NA ŚLUB [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN CZAS NA ŚLUB [720p]	
tvregionalna.pl.pl

PL: TVN KRYMINALNIE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN KRYMINALNIE [720p]	
tvregionalna.pl.pl

PL: TVN KULINARNE PODRÓŻE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN KULINARNE PODRÓŻE [720p]	
tvregionalna.pl.pl

PL: TVN KULTOWE SERIALE [[4K-Q]-Q]	
Polsat.Seriale.HD.pl

PL: TVN KULTOWE SERIALE [720p]	
Polsat.Seriale.HD.pl

PL: TVN MILIONERZY [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN MILIONERZY[720p]	
tvregionalna.pl.pl

PL: TVN MOMENTY PRAWDY [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN MOMENTY PRAWDY [720p]	
tvregionalna.pl.pl

PL: TVN MOTO [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN MOTO [720p]	
tvregionalna.pl.pl

PL: TVN PATROL ONLINE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN PATROL ONLINE [720p]	
tvregionalna.pl.pl

PL: TVN PRAWO I ŻYCIE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN PRAWO I ŻYCIE [720p]	
tvregionalna.pl.pl

PL: TVN RAJSKA MIŁOŚĆ [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN RAJSKA MIŁOŚĆ [720p]	
tvregionalna.pl.pl

PL: TVN REWOLUCJE W KUCHNI [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN REWOLUCJE W KUCHNI [720p]	
tvregionalna.pl.pl

PL: TVN SERIAL O KOBIETACH [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN SERIAL O KOBIETACH [720p]	
tvregionalna.pl.pl

PL: TVN SZKOŁA ŻYCIA [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN SZKOŁA ŻYCIA [720p]	
tvregionalna.pl.pl

PL: TVN SZPITALNE HISTORIE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN SZPITALNE HISTORIE [720p]	
tvregionalna.pl.pl

PL: TVN TALK-SHOW [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN TALK-SHOW [720p]	
tvregionalna.pl.pl

PL: TVN TELENOWELE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN TELENOWELE [720p]	
tvregionalna.pl.pl

PL: TVN USTERKA [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN USTERKA [720p]	
tvregionalna.pl.pl

PL: TVN W DOMU [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN W DOMU [720p]	
tvregionalna.pl.pl

PL: TVN ŻYCIE JAK W BAJCE [[4K-Q]-Q]	
tvregionalna.pl.pl

PL: TVN ŻYCIE JAK W BAJCE [720p]	
tvregionalna.pl.pl

PL: TVN24 [720p] (NA)	
tvregionalna.pl.pl

PL: TVP ABC 2 [720p]	
France.2.-.PL.pl

PL: TVP DOKUMENT [1080p]	
TVP.Dokument.HD.pl

PL: TVP HISTORIA 2 [720p]	
France.2.-.PL.pl

PL: TVP INFO [1080p]	
TVP.Info.HD.pl

PL: TVP KOBIETA [720p]	
TVP.Kobieta.HD.pl

PL: TVP POLONIA [720p]	
TVP.Polonia.HD.pl

PL: TVP POLONIA 1 [1080p]	
TVP.Polonia.HD.pl

PL: TVP ROZRYWKA (NA) [1080p]	
TVP.Rozrywka.pl

PL: TVP WORLD [720p]	
TVP.World.HD.pl

PL: TVP3 BIALYSTOK [1080p]	
tvregionalna.pl.pl

PL: TVP3 BYDGOSZCZ [1080p]	
TVP.3.Bydgoszcz.pl

PL: TVP3 GDANSK [1080p]	
tvregionalna.pl.pl

PL: TVP3 GORZOW [1080p]	
tvregionalna.pl.pl

PL: TVP3 KATOWICE [1080p]	
TVP.3.Katowice.pl

PL: TVP3 KIELCE [1080p]	
TVP.3.Kielce.pl

PL: TVP3 KRAKOW [1080p]	
tvregionalna.pl.pl

PL: TVP3 LODZ [1080p]	
tvregionalna.pl.pl

PL: TVP3 LUBLIN [1080p]	
TVP.3.Lublin.pl

PL: TVP3 OLSZTYN [1080p]	
TVP.3.Olsztyn.pl

PL: TVP3 OPOLE [1080p]	
TVP.3.Opole.pl

PL: TVP3 POZNAN [1080p]	
tvregionalna.pl.pl

PL: TVP3 RZESZOW [1080p]	
tvregionalna.pl.pl

PL: TVP3 SZCZECIN [1080p]	
TVP.3.Szczecin.pl

PL: TVP3 WARSZAWA [1080p]	
TVP.3.Warszawa.pl

PL: TVP3 WROCLAW (NA) [1080p]	
tvregionalna.pl.pl

PL: VOD 205 [1080p]	
tvregionalna.pl.pl

PL: VOD 206 [1080p]	
tvregionalna.pl.pl

PL: VOD 207 [1080p]	
tvregionalna.pl.pl

PL: VOD 208 [1080p]	
tvregionalna.pl.pl

PL: WALKI 1 (LIVE EVENT ONLY) [1080p]	
Polskie.Radio.Program.1.pl

PL: WALKI 2 (LIVE EVENT ONLY) [1080p]	
France.2.-.PL.pl

PL: WARNER TV [1080p]	
Warner.TV.HD.pl

PL: WODECKI KRAWCZYK I RYNKOWSKI KOLĘDY [1080p]	
tvregionalna.pl.pl

PL: WOTORE [1080p]	
tvregionalna.pl.pl

PL: WP [1080p]	
wPolsce.pl.pl

PL: WPOLSCE [1080p]	
wPolsce.pl.HD.pl

PL: WYDARZENIA 24 [1080p]	
Wydarzenia.24.HD.pl

PL: ZIMA RELAKS 1 [1080p]	
Polsat.Sport.Premium.1.pl

PL: ZIMA RELAKS 2 [1080p]	
France.2.-.PL.pl

PL: ZIMOWY KOMINEK 1 [1080p]	
Polskie.Radio.Program.1.pl

PL: ZŁOTE PRZEBOJE [1080p]	
tvregionalna.pl.pl

PLAY+: CANAL SPORT [1080p]	
Canal.beIN.Sport.en.Español.sv

PLAY+: CANAL+ [480p]	
CANAL+.Sport.5.HD.sk

PLAY+: CANAL+ EXTRA 1 [1080p]	
CANAL+.EXTRA.1.HD.pl

PLAY+: CANAL+ EXTRA 2 [1080p]	
CANAL+.EXTRA.2.HD.pl

PLAY+: CANAL+ EXTRA 3 [1080p]	
CANAL+.EXTRA.3.HD.pl

PLAY+: CANAL+ EXTRA 4 [1080p]	
CANAL+.EXTRA.4.HD.pl

PLAY+: CANAL+ EXTRA 5 [1080p]	
CANAL+.EXTRA.5.HD.pl

PLAY+: CANAL+ EXTRA 6 [1080p]	
CANAL+.EXTRA.6.HD.pl

PLAY+: CANAL+ EXTRA 7 [1080p]	
CANAL+.EXTRA.7.HD.pl

PLAY+: CANAL+ EXTRA 8 [1080p]	
CANAL+.EXTRA.8.HD.pl

PLAY+: CANAL+ EXTRA 9 [1080p]	
CANAL+.EXTRA.9.HD.pl

PLAY+: CANAL+ SPORT 2 [1080p]	
TV.2.Sport.2.Norway.(NO,NO).no

PLAY+: CANAL+ SPORT 3 [1080p]	
CANAL+.Sport.3.HD.pl

PLAY+: CANAL+ SPORT 4 [1080p]	
CANAL+.Sport.4.HD.sk

PLAY+: CANAL+ SPORT 5 [1080p]	
CANAL+.Sport.5.HD.sk

PLAY+: ELEVEN SPORTS 1 [1080p]	
Eleven.Sports.1.HD.pl

PLAY+: ELEVEN SPORTS 2 [1080p]	
Eleven.Sports.2.HD.pl

PLAY+: ELEVEN SPORTS 3 [1080p]	
Eleven.Sports.3.HD.pl

PLAY+: ELEVEN SPORTS 4 [1080p]	
Eleven.Sports.4.HD.pl

PLAY+: EVENT 1 [1080p]	
CANAL+.Sport.Event.1.ch

PLAY+: EVENT 2 [1080p]	
CANAL+.Sport.Event.2.ch

PLAY+: EVENT 3 [1080p]	
ESPN.3.HD.(ESPN.3.HD).pe

PLAY+: EVENT 4 [1080p]	
EVENTOS.4.HD..uy
"""

channels = set()
for line in prompt.splitlines():
    if line.startswith("PL:") or line.startswith("PL-") or line.startswith("PLAY+:"):
        c = line.split("\t")[0].strip()
        channels.add(c)

with open("pl_epg.txt", "r") as f:
    epgs = f.read()

with open("pl_epg_mapping_task.md", "w") as f:
    f.write("# EPG Mapping Task for Polish Channels\n\n")
    f.write("I have a list of IPTV channels and a list of available EPG channels. Please act as a mapping expert and map each IPTV channel to the exact ID of the closest available EPG channel. If there is no logical match, skip the channel entirely. Do NOT guess or hallucinate IDs.\n\n")
    f.write("Output format should be a valid JSON object where the key is the exact IPTV channel name (including [1080p] etc.) and the value is the matched EPG ID.\n\n")
    f.write("## Available EPGs\n")
    f.write(epgs)
    f.write("\n## IPTV Channels to Map\n")
    for c in sorted(list(channels)):
        f.write(f"- {c}\n")

print("Generated pl_epg_mapping_task.md")
