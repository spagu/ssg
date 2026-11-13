// Export layouts as widely used mailing tools write them: realistic column
// sets, invented people, example.com addresses. Each fixture says which header
// should land in which role, and which headers are ignored as names.

export interface Layout {
  label: string;
  csv: string;
  columns: Record<string, string | null>; // role → expected header
  names: string[]; // headers reported as "names are not stored"
  ambiguous?: { role: string; chosen: string; alsoMatched: string[] }[];
}

const none = { status: null, signedUpAt: null, unsubscribedAt: null, bouncedAt: null, language: null, tags: null, sourcePage: null };

export const LAYOUTS: Layout[] = [
  {
    label: "audience export: opt-in and confirm times, member rating, geo columns",
    csv: [
      "Email Address,First Name,Last Name,MEMBER_RATING,OPTIN_TIME,OPTIN_IP,CONFIRM_TIME,CONFIRM_IP,LATITUDE,LONGITUDE,TIMEZONE,CC,LAST_CHANGED,LEID,NOTES,TAGS",
      'ana@example.com,Ana,Nowak,2,2024-03-01 10:00:00,203.0.113.5,2024-03-01 10:05:00,203.0.113.5,,,Europe/Warsaw,PL,2024-05-01 00:00:00,123,,"Customers,VIP"',
    ].join("\n"),
    columns: { ...none, email: "Email Address", signedUpAt: "OPTIN_TIME", tags: "TAGS" },
    names: ["First Name", "Last Name"],
    ambiguous: [{ role: "signedUpAt", chosen: "OPTIN_TIME", alsoMatched: ["CONFIRM_TIME"] }],
  },
  {
    label: "unsubscribed export: unsubscribe time and campaign columns",
    csv: "Email Address,First Name,OPTIN_TIME,UNSUB_TIME,UNSUB_CAMPAIGN_TITLE,UNSUB_REASON\nben@example.com,Ben,2023-01-02 08:00:00,2024-02-03 09:00:00,Spring,No longer interested\n",
    columns: { ...none, email: "Email Address", signedUpAt: "OPTIN_TIME", unsubscribedAt: "UNSUB_TIME" },
    names: ["First Name"],
  },
  {
    label: "cleaned export: clean time",
    csv: "Email Address,CLEAN_TIME,CLEAN_CAMPAIGN_TITLE\ncid@example.com,2024-04-05 06:07:08,Spring\n",
    columns: { ...none, email: "Email Address", bouncedAt: "CLEAN_TIME" },
    names: [],
  },
  {
    label: "subscriber list: snake_case with status, dates and segments",
    csv: "id,email,first_name,last_name,status,created_at,updated_at,unsubscribed_at,bounced_at,signup_source,language,segments\n1,dan@example.com,Dan,Lee,active,2024-01-01T00:00:00Z,2024-01-02T00:00:00Z,,,https://example.com/blog/post?x=1,en_US,news;offers\n",
    columns: {
      email: "email", status: "status", signedUpAt: "created_at", unsubscribedAt: "unsubscribed_at", bouncedAt: "bounced_at",
      language: "language", tags: "segments", sourcePage: "signup_source",
    },
    names: ["first_name", "last_name"],
  },
  {
    label: "European spreadsheet: semicolons, E-mail, date subscribed, groups",
    csv: "E-mail;Name;Status;Date subscribed;Language;Groups\neva@example.com;Eva;Subscribed;01.02.2024;pl;weekly\n",
    columns: { ...none, email: "E-mail", status: "Status", signedUpAt: "Date subscribed", language: "Language", tags: "Groups" },
    names: ["Name"],
  },
  {
    label: "tab-separated: subscriber email, subscribed, locale, lists, referrer",
    csv: "Subscriber Email\tSubscribed On\tSubscriber Status\tLocale\tLists\tReferrer\tCountry\nfay@example.com\t1711965600\tconfirmed\tde\tmain\t/signup/\tDE\n",
    columns: {
      ...none, email: "Subscriber Email", status: "Subscriber Status", signedUpAt: "Subscribed On", language: "Locale", tags: "Lists", sourcePage: "Referrer",
    },
    names: [],
  },
  {
    label: "contacts export: full name, opt-in date, list and source",
    csv: "Contact Email,Full Name,Email Status,Opt-in Date,List,Source,Phone\ngus@example.com,Gus Grey,unconfirmed,2024/05/06,members,Landing form,555\n",
    columns: { ...none, email: "Contact Email", status: "Email Status", signedUpAt: "Opt-in Date", tags: "List", sourcePage: "Source" },
    names: ["Full Name"],
  },
];
