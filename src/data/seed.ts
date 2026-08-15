import type { League } from '@/api/types';

/**
 * Seed data for the demo dataset.
 *
 * Coverage goes well beyond the PRD's "3-5 leagues" MVP floor. East Africa is
 * kept as its own region rather than folded into Africa, because depth there is
 * the stated differentiator (PRD 1.3) and it needs to be filterable on its own.
 */

export const LEAGUES: League[] = [
  // ---- Europe ----
  { id: 'eng-pl', name: 'Premier League', shortName: 'EPL', country: 'England', countryCode: 'ENG', tier: 1, region: 'europe' },
  { id: 'esp-laliga', name: 'LaLiga', shortName: 'LAL', country: 'Spain', countryCode: 'ESP', tier: 1, region: 'europe' },
  { id: 'ita-seriea', name: 'Serie A', shortName: 'SEA', country: 'Italy', countryCode: 'ITA', tier: 1, region: 'europe' },
  { id: 'ger-bundesliga', name: 'Bundesliga', shortName: 'BUN', country: 'Germany', countryCode: 'GER', tier: 1, region: 'europe' },
  { id: 'fra-ligue1', name: 'Ligue 1', shortName: 'LI1', country: 'France', countryCode: 'FRA', tier: 1, region: 'europe' },
  { id: 'ned-eredivisie', name: 'Eredivisie', shortName: 'ERE', country: 'Netherlands', countryCode: 'NED', tier: 1, region: 'europe' },
  { id: 'por-primeira', name: 'Primeira Liga', shortName: 'PRI', country: 'Portugal', countryCode: 'POR', tier: 1, region: 'europe' },
  { id: 'tur-superlig', name: 'Süper Lig', shortName: 'SUP', country: 'Türkiye', countryCode: 'TUR', tier: 1, region: 'europe' },
  { id: 'eng-championship', name: 'EFL Championship', shortName: 'CHA', country: 'England', countryCode: 'ENG', tier: 2, region: 'europe' },

  // ---- East Africa (the differentiator) ----
  { id: 'uga-upl', name: 'Uganda Premier League', shortName: 'UPL', country: 'Uganda', countryCode: 'UGA', tier: 1, region: 'east-africa' },
  { id: 'ken-fkfpl', name: 'FKF Premier League', shortName: 'FKF', country: 'Kenya', countryCode: 'KEN', tier: 1, region: 'east-africa' },
  { id: 'tza-ligikuu', name: 'Ligi Kuu Bara', shortName: 'LKB', country: 'Tanzania', countryCode: 'TZA', tier: 1, region: 'east-africa' },
  { id: 'rwa-premier', name: 'Rwanda Premier League', shortName: 'RPL', country: 'Rwanda', countryCode: 'RWA', tier: 1, region: 'east-africa' },

  // ---- Rest of Africa ----
  { id: 'egy-premier', name: 'Egyptian Premier League', shortName: 'EGY', country: 'Egypt', countryCode: 'EGY', tier: 1, region: 'africa' },
  { id: 'rsa-premiership', name: 'Betway Premiership', shortName: 'RSA', country: 'South Africa', countryCode: 'RSA', tier: 1, region: 'africa' },
  { id: 'nga-npfl', name: 'Nigeria Premier Football League', shortName: 'NPF', country: 'Nigeria', countryCode: 'NGA', tier: 1, region: 'africa' },

  // ---- Americas ----
  { id: 'bra-seriea', name: 'Brasileirão Série A', shortName: 'BRA', country: 'Brazil', countryCode: 'BRA', tier: 1, region: 'americas' },
  { id: 'usa-mls', name: 'Major League Soccer', shortName: 'MLS', country: 'United States', countryCode: 'USA', tier: 1, region: 'americas' },
];

/**
 * Team rosters as [name, shortName], listed strongest-first. The generator
 * turns list position into a latent strength rating, so ordering here decides
 * roughly where a club lands in the simulated table.
 *
 * Short names must be unique within a league — they form the team id.
 * `assertUniqueShortNames` below enforces that.
 */
export const TEAM_ROSTERS: Record<string, [string, string][]> = {
  'eng-pl': [
    ['Manchester City', 'MCI'], ['Arsenal', 'ARS'], ['Liverpool', 'LIV'], ['Chelsea', 'CHE'],
    ['Tottenham Hotspur', 'TOT'], ['Newcastle United', 'NEW'], ['Aston Villa', 'AVL'],
    ['Manchester United', 'MUN'], ['Brighton & Hove Albion', 'BHA'], ['West Ham United', 'WHU'],
    ['Crystal Palace', 'CRY'], ['Brentford', 'BRE'], ['Fulham', 'FUL'], ['Nottingham Forest', 'NFO'],
    ['Everton', 'EVE'], ['Wolverhampton Wanderers', 'WOL'], ['AFC Bournemouth', 'BOU'],
    ['Leeds United', 'LEE'], ['Burnley', 'BUR'], ['Sunderland', 'SUN'],
  ],
  'esp-laliga': [
    ['Real Madrid', 'RMA'], ['FC Barcelona', 'BAR'], ['Atlético Madrid', 'ATM'], ['Athletic Club', 'ATH'],
    ['Real Sociedad', 'RSO'], ['Villarreal', 'VIL'], ['Real Betis', 'BET'], ['Sevilla', 'SEV'],
    ['Valencia', 'VAL'], ['Girona', 'GIR'], ['CA Osasuna', 'OSA'], ['Celta Vigo', 'CEL'],
    ['Rayo Vallecano', 'RAY'], ['RCD Mallorca', 'MLL'], ['Getafe', 'GET'], ['Deportivo Alavés', 'ALA'],
    ['RCD Espanyol', 'ESP'], ['Levante', 'LEV'], ['Elche', 'ELC'], ['Real Oviedo', 'OVI'],
  ],
  'ita-seriea': [
    ['Inter', 'INT'], ['Napoli', 'NAP'], ['AC Milan', 'MIL'], ['Juventus', 'JUV'], ['Atalanta', 'ATA'],
    ['AS Roma', 'ROM'], ['Lazio', 'LAZ'], ['Fiorentina', 'FIO'], ['Bologna', 'BOL'], ['Torino', 'TOR'],
    ['Udinese', 'UDI'], ['Genoa', 'GEN'], ['Como', 'COM'], ['Cagliari', 'CAG'], ['Hellas Verona', 'VER'],
    ['Parma', 'PAR'], ['Lecce', 'LEC'], ['Sassuolo', 'SAS'], ['Pisa', 'PIS'], ['Cremonese', 'CRE'],
  ],
  'ger-bundesliga': [
    ['Bayern Munich', 'BAY'], ['Bayer Leverkusen', 'LEV'], ['RB Leipzig', 'RBL'],
    ['Borussia Dortmund', 'BVB'], ['VfB Stuttgart', 'STU'], ['Eintracht Frankfurt', 'SGE'],
    ['TSG Hoffenheim', 'HOF'], ['SC Freiburg', 'SCF'], ['Werder Bremen', 'BRE'], ['Union Berlin', 'FCU'],
    ['VfL Wolfsburg', 'WOB'], ['Mainz 05', 'M05'], ['Borussia Mönchengladbach', 'BMG'],
    ['FC Augsburg', 'FCA'], ['FC St. Pauli', 'STP'], ['1. FC Heidenheim', 'HDH'],
    ['Hamburger SV', 'HSV'], ['1. FC Köln', 'KOE'],
  ],
  'fra-ligue1': [
    ['Paris Saint-Germain', 'PSG'], ['Marseille', 'OM'], ['AS Monaco', 'ASM'], ['Lille', 'LIL'],
    ['OGC Nice', 'NIC'], ['Olympique Lyonnais', 'OL'], ['RC Lens', 'LEN'], ['Stade Rennais', 'REN'],
    ['RC Strasbourg', 'STR'], ['Stade Brestois', 'BRS'], ['Toulouse', 'TFC'], ['FC Nantes', 'NAN'],
    ['AJ Auxerre', 'AUX'], ['Angers SCO', 'ANG'], ['Le Havre', 'HAC'], ['FC Metz', 'MET'],
    ['FC Lorient', 'LOR'], ['Paris FC', 'PFC'],
  ],
  'ned-eredivisie': [
    ['PSV Eindhoven', 'PSV'], ['Ajax', 'AJA'], ['Feyenoord', 'FEY'], ['AZ Alkmaar', 'AZ'],
    ['FC Twente', 'TWE'], ['FC Utrecht', 'UTR'], ['Go Ahead Eagles', 'GAE'], ['SC Heerenveen', 'HEE'],
    ['NEC Nijmegen', 'NEC'], ['Sparta Rotterdam', 'SPA'], ['Fortuna Sittard', 'FOR'],
    ['PEC Zwolle', 'ZWO'], ['Heracles Almelo', 'HER'], ['Willem II', 'WIL'], ['FC Groningen', 'GRO'],
    ['NAC Breda', 'NAC'], ['Excelsior', 'EXC'], ['Telstar', 'TEL'],
  ],
  'por-primeira': [
    ['Sporting CP', 'SCP'], ['Benfica', 'BEN'], ['FC Porto', 'POR'], ['SC Braga', 'BRA'],
    ['Vitória SC', 'VIT'], ['Moreirense', 'MOR'], ['Famalicão', 'FAM'], ['Santa Clara', 'STC'],
    ['Estoril Praia', 'EST'], ['Gil Vicente', 'GIL'], ['Casa Pia', 'CAS'], ['Rio Ave', 'RIO'],
    ['FC Arouca', 'ARO'], ['Estrela da Amadora', 'EDA'], ['CD Nacional', 'NAC'], ['AVS', 'AVS'],
    ['Tondela', 'TON'], ['FC Alverca', 'ALV'],
  ],
  'tur-superlig': [
    ['Galatasaray', 'GAL'], ['Fenerbahçe', 'FEN'], ['Beşiktaş', 'BJK'], ['Trabzonspor', 'TRA'],
    ['Başakşehir', 'IBF'], ['Samsunspor', 'SAM'], ['Göztepe', 'GOZ'], ['Eyüpspor', 'EYU'],
    ['Kasımpaşa', 'KAS'], ['Antalyaspor', 'ANT'], ['Alanyaspor', 'ALA'], ['Çaykur Rizespor', 'RIZ'],
    ['Konyaspor', 'KON'], ['Gaziantep FK', 'GAZ'], ['Kayserispor', 'KAY'], ['Sivasspor', 'SIV'],
    ['Fatih Karagümrük', 'KAR'], ['Kocaelispor', 'KOC'],
  ],
  'eng-championship': [
    ['Leicester City', 'LEI'], ['Southampton', 'SOU'], ['Ipswich Town', 'IPS'], ['Middlesbrough', 'MID'],
    ['West Bromwich Albion', 'WBA'], ['Coventry City', 'COV'], ['Norwich City', 'NOR'],
    ['Sheffield United', 'SHU'], ['Millwall', 'MIL'], ['Bristol City', 'BRC'], ['Hull City', 'HUL'],
    ['Preston North End', 'PNE'], ['Watford', 'WAT'], ['Blackburn Rovers', 'BLA'], ['Swansea City', 'SWA'],
    ['Queens Park Rangers', 'QPR'], ['Sheffield Wednesday', 'SHW'], ['Stoke City', 'STK'],
    ['Derby County', 'DER'], ['Portsmouth', 'POR'], ['Oxford United', 'OXF'], ['Charlton Athletic', 'CHA'],
    ['Wrexham', 'WRE'], ['Birmingham City', 'BIR'],
  ],

  'uga-upl': [
    ['Vipers SC', 'VIP'], ['KCCA FC', 'KCC'], ['SC Villa', 'VIL'], ['URA FC', 'URA'], ['BUL FC', 'BUL'],
    ['Express FC', 'EXP'], ['Kitara FC', 'KIT'], ['NEC FC', 'NEC'], ['Police FC', 'POL'],
    ['Mbarara City', 'MBA'], ['Maroons FC', 'MAR'], ['Bright Stars FC', 'BRS'], ['Gaddafi FC', 'GAD'],
    ['Lugazi FC', 'LUG'], ['Kyetume FC', 'KYE'], ['Onduparaka FC', 'OND'],
  ],
  'ken-fkfpl': [
    ['Gor Mahia', 'GOR'], ['Tusker FC', 'TUS'], ['Kakamega Homeboyz', 'KAK'], ['AFC Leopards', 'AFC'],
    ['Bandari FC', 'BAN'], ['KCB FC', 'KCB'], ['Ulinzi Stars', 'ULI'], ['Kenya Police FC', 'KPO'],
    ['Posta Rangers', 'POS'], ['Sofapaka', 'SOF'], ['Nairobi City Stars', 'NCS'],
    ['Kariobangi Sharks', 'KSH'], ["Murang'a Seal", 'MUR'], ['Shabana FC', 'SHA'],
    ['Mathare United', 'MAT'], ['Bidco United', 'BID'], ['Talanta FC', 'TAL'], ['Mara Sugar FC', 'MSU'],
  ],
  'tza-ligikuu': [
    ['Young Africans', 'YAN'], ['Simba SC', 'SIM'], ['Azam FC', 'AZA'], ['Coastal Union', 'COA'],
    ['Namungo FC', 'NAM'], ['KMC FC', 'KMC'], ['Singida Black Stars', 'SBS'],
    ['Tanzania Prisons', 'PRI'], ['Mtibwa Sugar', 'MTI'], ['Dodoma Jiji', 'DOD'],
    ['Fountain Gate', 'FGA'], ['Ihefu FC', 'IHE'], ['Kagera Sugar', 'KAG'], ['JKT Tanzania', 'JKT'],
    ['Pamba Jiji', 'PAM'], ['Mashujaa FC', 'MAS'],
  ],
  'rwa-premier': [
    ['APR FC', 'APR'], ['Rayon Sports', 'RAY'], ['Rwanda Police FC', 'RPO'], ['AS Kigali', 'ASK'],
    ['Musanze FC', 'MUS'], ['Gasogi United', 'GAS'], ['Bugesera FC', 'BUG'], ['Etincelles FC', 'ETI'],
    ['Mukura Victory Sports', 'MUK'], ['Marines FC', 'MRN'], ['Rutsiro FC', 'RUT'],
    ['Gorilla FC', 'GOR'], ['Amagaju FC', 'AMA'], ['Interforce FC', 'INT'], ['Vision FC', 'VIS'],
    ['Sunrise FC', 'SUN'],
  ],

  'egy-premier': [
    ['Al Ahly', 'AHL'], ['Zamalek', 'ZAM'], ['Pyramids FC', 'PYR'], ['Al Masry', 'MAS'],
    ['Ismaily', 'ISM'], ['Ceramica Cleopatra', 'CER'], ['Smouha', 'SMO'], ['ENPPI', 'ENP'],
    ['National Bank', 'NBE'], ['Future FC', 'FUT'], ['Al Ittihad Alexandria', 'ITT'],
    ['Baladiyat El Mahalla', 'BEM'], ['ZED FC', 'ZED'], ['Pharco FC', 'PHA'], ['Modern Sport', 'MOD'],
    ['El Gouna', 'GOU'], ['Petrojet', 'PET'], ['Haras El Hodoud', 'HAR'],
  ],
  'rsa-premiership': [
    ['Mamelodi Sundowns', 'SUN'], ['Orlando Pirates', 'PIR'], ['Kaizer Chiefs', 'CHI'],
    ['Stellenbosch FC', 'STE'], ['SuperSport United', 'SSU'], ['Sekhukhune United', 'SEK'],
    ['Golden Arrows', 'ARR'], ['Polokwane City', 'POL'], ['AmaZulu', 'AMA'], ['Chippa United', 'CHP'],
    ['Richards Bay', 'RIC'], ['Marumo Gallants', 'MAR'], ['TS Galaxy', 'GAL'], ['Magesi FC', 'MAG'],
    ['Royal AM', 'ROY'], ['Cape Town City', 'CTC'],
  ],
  'nga-npfl': [
    ['Enyimba', 'ENY'], ['Rivers United', 'RIV'], ['Remo Stars', 'REM'], ['Rangers International', 'RAN'],
    ['Plateau United', 'PLA'], ['Lobi Stars', 'LOB'], ['Bendel Insurance', 'BEN'],
    ['Shooting Stars', 'SHO'], ['Nasarawa United', 'NAS'], ['Katsina United', 'KAT'],
    ['Akwa United', 'AKW'], ['Abia Warriors', 'ABI'], ['Kwara United', 'KWA'], ['Sunshine Stars', 'SUN'],
    ['Bayelsa United', 'BAY'], ['Heartland', 'HEA'], ['El-Kanemi Warriors', 'ELK'],
    ['Niger Tornadoes', 'NIG'], ['Ikorodu City', 'IKO'], ['Kano Pillars', 'KAN'],
  ],

  'bra-seriea': [
    ['Flamengo', 'FLA'], ['Palmeiras', 'PAL'], ['Botafogo', 'BOT'], ['Fluminense', 'FLU'],
    ['São Paulo', 'SAO'], ['Internacional', 'INT'], ['Grêmio', 'GRE'], ['Corinthians', 'COR'],
    ['Cruzeiro', 'CRU'], ['Atlético Mineiro', 'CAM'], ['Bahia', 'BAH'], ['Fortaleza', 'FOR'],
    ['Vasco da Gama', 'VAS'], ['Red Bull Bragantino', 'RBB'], ['Santos', 'SAN'], ['Vitória', 'VIT'],
    ['Juventude', 'JUV'], ['Mirassol', 'MIR'], ['Sport Recife', 'SPT'], ['Ceará', 'CEA'],
  ],
  'usa-mls': [
    ['Inter Miami', 'MIA'], ['Los Angeles FC', 'LAF'], ['Columbus Crew', 'CLB'],
    ['Philadelphia Union', 'PHI'], ['FC Cincinnati', 'CIN'], ['Seattle Sounders', 'SEA'],
    ['LA Galaxy', 'LAG'], ['Orlando City', 'ORL'], ['New York Red Bulls', 'RBNY'],
    ['New York City FC', 'NYC'], ['Atlanta United', 'ATL'], ['Portland Timbers', 'POR'],
    ['Real Salt Lake', 'RSL'], ['Minnesota United', 'MIN'], ['Vancouver Whitecaps', 'VAN'],
    ['Nashville SC', 'NSH'], ['Austin FC', 'AUS'], ['Charlotte FC', 'CLT'], ['Chicago Fire', 'CHI'],
    ['Toronto FC', 'TOR'],
  ],
};

/**
 * Venue scoring baselines used to simulate results. African top flights are
 * modelled as lower scoring and the Eredivisie/Bundesliga as higher, which is
 * broadly what the real data shows.
 */
export const LEAGUE_SCORING: Record<string, { home: number; away: number }> = {
  'eng-pl': { home: 1.56, away: 1.24 },
  'esp-laliga': { home: 1.48, away: 1.12 },
  'ita-seriea': { home: 1.51, away: 1.19 },
  'ger-bundesliga': { home: 1.72, away: 1.38 },
  'fra-ligue1': { home: 1.54, away: 1.21 },
  'ned-eredivisie': { home: 1.78, away: 1.42 },
  'por-primeira': { home: 1.44, away: 1.06 },
  'tur-superlig': { home: 1.62, away: 1.24 },
  'eng-championship': { home: 1.45, away: 1.16 },
  'uga-upl': { home: 1.24, away: 0.86 },
  'ken-fkfpl': { home: 1.19, away: 0.83 },
  'tza-ligikuu': { home: 1.22, away: 0.84 },
  'rwa-premier': { home: 1.2, away: 0.82 },
  'egy-premier': { home: 1.28, away: 0.92 },
  'rsa-premiership': { home: 1.26, away: 0.94 },
  'nga-npfl': { home: 1.34, away: 0.72 },
  'bra-seriea': { home: 1.42, away: 1.02 },
  'usa-mls': { home: 1.66, away: 1.28 },
};

/**
 * Team ids are derived from short names, so a duplicate inside one league would
 * silently collapse two clubs into one. Fail loudly instead.
 */
function assertRosterIntegrity() {
  for (const league of LEAGUES) {
    const roster = TEAM_ROSTERS[league.id];
    if (!roster) throw new Error(`No roster for league ${league.id}`);
    if (roster.length < 8 || roster.length % 2 !== 0) {
      throw new Error(`League ${league.id} needs an even roster of 8+, got ${roster.length}`);
    }
    const seen = new Set<string>();
    for (const [name, short] of roster) {
      const key = short.toLowerCase();
      if (seen.has(key)) {
        throw new Error(`Duplicate short name "${short}" in ${league.id} (${name})`);
      }
      seen.add(key);
    }
    if (!LEAGUE_SCORING[league.id]) {
      throw new Error(`No scoring baseline for league ${league.id}`);
    }
  }
}

assertRosterIntegrity();

/** Version tag written onto every prediction (FR-13). */
export const MODEL_VERSION = 'poisson-1.2.0';
