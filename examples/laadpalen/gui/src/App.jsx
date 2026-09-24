import { useState } from "react";
import { GEBRUIKERS, ADRESSEN } from "./data.js";

export default function App() {
  const [user, setUser] = useState(GEBRUIKERS[0].id);
  const [postcode, setPostcode] = useState("1111AA");
  const [huisnummer, setHuisnummer] = useState("1");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);
  const [rawRequest, setRawRequest] = useState("-");
  const [rawResponse, setRawResponse] = useState("-");

  const selectedUser = GEBRUIKERS.find((g) => g.id === user);

  async function vraagAan() {
    setError(null);
    setResult(null);
    setLoading(true);

    try {
      const res = await fetch("/api/request-laadpaal", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user, postcode: postcode.trim(), huisnummer }),
      });

      const data = await res.json();
      setRawRequest(JSON.stringify(data.debug?.request, null, 2));
      // show the real, complete PDP response here - unmodified. This panel's whole
      // purpose is showing what the API actually got back from the authorizer.
      setRawResponse(JSON.stringify(data.debug?.response, null, 2));

      if (data.error) {
        throw new Error(data.error);
      }

      setResult({ allowed: data.allowed, reason: data.reason });
    } catch (err) {
      setRawResponse(`Fout: ${err.message}`);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main>
      <header>
        <h1>Laadpaal aanvraag</h1>
        <p>
          Test-GUI voor het laadpalen-voorbeeld — roept de laadpalen-API aan, die op zijn beurt de echte
          PDP (Eamerald authorizer) aanroept via <code>/access/v1/evaluation</code>.
        </p>
      </header>

      {error && (
        <div className="error show">
          Kon de laadpalen-API niet bereiken ({error}). Controleer of <code>make laadpalen-api</code> en de
          deployment draaien (<code>kubectl -n eamerald get pods</code>).
        </div>
      )}

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
