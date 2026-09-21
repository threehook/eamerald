import { useState } from "react";
import { GEBRUIKERS, ADRESSEN } from "./data.js";

export default function App() {
  const [baseUrl, setBaseUrl] = useState("https://localhost:8383");
  const [user, setUser] = useState(GEBRUIKERS[0].id);
  const [postcode, setPostcode] = useState("1111AA");
  const [huisnummer, setHuisnummer] = useState("1");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [showCertHint, setShowCertHint] = useState(false);
  const [rawRequest, setRawRequest] = useState("-");
  const [rawResponse, setRawResponse] = useState("-");

  const selectedUser = GEBRUIKERS.find((g) => g.id === user);

  async function vraagAan() {
    setShowCertHint(false);
    setResult(null);
    setLoading(true);

    const url = baseUrl.replace(/\/$/, "");
    const body = {
      subject: { type: "user", id: user },
      action: { name: "request_laadpaal" },
      resource: {
        type: "adres",
        properties: { postcode: postcode.trim(), huisnummer: Number(huisnummer) },
      },
      context: { doelbinding: "laadpalen" },
    };

    setRawRequest(`POST ${url}/access/v1/evaluation\n${JSON.stringify(body, null, 2)}`);

    try {
      const res = await fetch(`${url}/access/v1/evaluation`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }

      const data = await res.json();
      // show the real, complete Eamerald response here - unmodified. This panel's
      // whole purpose is showing what the API actually returned.
      setRawResponse(JSON.stringify(data, null, 2));

      if (typeof data.decision !== "boolean") {
        throw new Error("Onverwachte response-vorm (geen 'decision' veld gevonden)");
      }

      setResult({
        allowed: data.decision === true,
        reason: data.context?.reason ?? "",
      });
    } catch (err) {
      setRawResponse(`Fout: ${err.message}`);
      setShowCertHint(true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main>
      <header>
        <h1>Laadpaal aanvraag</h1>
        <p>
          Test-GUI voor het laadpalen-voorbeeld — roept de echte PDP (Eamerald authorizer) rechtstreeks
          aan via <code>/access/v1/evaluation</code>.
        </p>
      </header>

      {showCertHint && (
        <div className="error show">
          Kon de authorizer niet bereiken. Meestal komt dit door het zelfondertekende certificaat: open{" "}
          <a href={baseUrl} target="_blank" rel="noopener noreferrer">
            de authorizer-URL
          </a>{" "}
          eenmaal rechtstreeks in een nieuwe tab en accepteer de waarschuwing, probeer het dan hier
          opnieuw. Controleer ook of de deployment draait (<code>kubectl -n eamerald get pods</code>).
        </div>
      )}

      <div className="card">
        <h2>Instellingen</h2>
        <div className="field">
          <label htmlFor="base-url">Authorizer-URL</label>
          <input
            type="text"
            id="base-url"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
          />
        </div>
      </div>

      <div className="card">
        <h2>Wie vraagt aan?</h2>
        <div className="field">
          <label htmlFor="user">Gebruiker</label>
          <select id="user" value={user} onChange={(e) => setUser(e.target.value)}>
            {GEBRUIKERS.map((g) => (
              <option key={g.id} value={g.id}>
                {g.naam}
              </option>
            ))}
          </select>
        </div>
        <p className="muted">{selectedUser?.info}</p>
      </div>

      <div className="card">
        <h2>Voor welk adres?</h2>
        <div className="row">
          <div className="field">
            <label htmlFor="postcode">Postcode</label>
            <input
              type="text"
              id="postcode"
              value={postcode}
              onChange={(e) => setPostcode(e.target.value)}
            />
          </div>
          <div className="field narrow">
            <label htmlFor="huisnummer">Huisnummer</label>
            <input
              type="text"
              id="huisnummer"
              value={huisnummer}
              onChange={(e) => setHuisnummer(e.target.value)}
            />
          </div>
        </div>
        <div className="chips">
          {ADRESSEN.map((a) => {
            const active = postcode === a.postcode && Number(huisnummer) === a.huisnummer;
            return (
              <button
                key={`${a.postcode}-${a.huisnummer}`}
                type="button"
                className={`chip${active ? " active" : ""}`}
                onClick={() => {
                  setPostcode(a.postcode);
                  setHuisnummer(String(a.huisnummer));
                }}
              >
                {a.label}
              </button>
            );
          })}
        </div>
      </div>

      <button className="primary" onClick={vraagAan} disabled={loading}>
        {loading ? "Bezig..." : "Toets aanvraag"}
      </button>

      {result && (
        <div className={`result show ${result.allowed ? "allow" : "deny"}`}>
          <span className="badge">{result.allowed ? "✅" : "❌"}</span>
          <span className="text">
            <strong>{result.allowed ? "Toegekend" : "Niet toegekend"}</strong>
            <span>{result.reason}</span>
          </span>
        </div>
      )}

      <details>
        <summary>Toon API-aanroep (request/response)</summary>
        <pre>{rawRequest}</pre>
        <pre>{rawResponse}</pre>
      </details>
    </main>
  );
}
