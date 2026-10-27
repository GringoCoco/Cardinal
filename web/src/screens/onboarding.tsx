import { useState } from "preact/hooks";
import { pair, Preferences } from "../api";
import { Button, Card, ErrorBanner, Header, Screen, Toggle } from "../components/ui";

type Step = "welcome" | "pair" | "providers" | "done";

export function OnboardingScreen({ preferences, update, complete }: { preferences: Preferences; update: (next: Preferences) => void; complete: (device: string) => void }) {
  const [step, setStep] = useState<Step>("welcome");
  const [code, setCode] = useState("");
  const [device, setDevice] = useState("macbook");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const connect = async () => {
    setBusy(true); setError("");
    try {
      const result = await pair(code.replace(/\s/g, ""));
      setDevice(result.name || result.device);
      setStep("providers");
    } catch (e) { setError((e as Error).message); setBusy(false); }
  };
  const toggle = (provider: string) => update({ ...preferences, providers: { ...preferences.providers, [provider]: !preferences.providers[provider] } });
  return (
    <Screen className="onboarding">
      <Header title={step === "welcome" ? "Cardinal" : step === "done" ? "Подключение" : step === "providers" ? "Провайдеры" : "Подключение"} onBack={step === "welcome" ? undefined : () => setStep(step === "providers" ? "pair" : "welcome")} />
      <main className="content">
        {step === "welcome" && <>
          <div className="brand-mark">&gt;_ Cardinal</div>
          <h1>Ваши агенты — всегда под рукой.</h1>
          <p className="lead">Следите с телефона за терминальными сессиями, которые уже запущены на вашем компьютере.</p>
          <Card className="terminal-hint"><code>КОМПЬЮТЕР + КОННЕКТОР + ТЕЛЕФОН</code><p>Cardinal передаёт вывод, сообщения и запросы решений через защищённый канал.</p></Card>
          <ol className="steps"><li>Наблюдайте за ходом работы</li><li>Подтверждайте команды осознанно</li><li>Отправляйте агенту инструкции</li></ol>
          <Button variant="primary" onClick={() => setStep("pair")}>Подключить компьютер</Button>
        </>}
        {step === "pair" && <>
          <div className="eyebrow green">Сопряжение / шаг 1 из 3</div>
          <h1>Подключите свой компьютер</h1>
          <p className="lead">На компьютере выполните команду:</p>
          <Card className="terminal-hint"><code>$ cardinald pair-code</code><p>Коннектор покажет одноразовый код. Он действует 10 минут.</p></Card>
          <label className="field-label">Код сопряжения</label>
          <input className="pair-code" value={code} inputMode="numeric" maxLength={7} placeholder="000 000" onInput={(e) => setCode((e.target as HTMLInputElement).value)} />
          <Button variant="primary" disabled={busy || code.replace(/\s/g, "").length !== 6} onClick={connect}>Подключить</Button>
          <ErrorBanner text={error} />
        </>}
        {step === "providers" && <>
          <div className="eyebrow green">Источники / шаг 2 из 3</div>
          <h1>Каких агентов отслеживать?</h1>
          <p className="lead">Найдены доступные источники. Выбор можно изменить позднее.</p>
          {Object.keys(preferences.providers).map((provider) => <Toggle checked={preferences.providers[provider]} onChange={() => toggle(provider)} label={provider} hint="Обнаружен на компьютере" />)}
          <Button variant="primary" onClick={() => setStep("done")}>Продолжить</Button>
        </>}
        {step === "done" && <>
          <div className="success-glyph">[✓]</div>
          <h1>Компьютер подключён</h1>
          <Card><strong>{device}</strong><p>Коннектор на связи. Защищённый канал установлен.</p></Card>
          <Button variant="primary" onClick={() => complete(device)}>К сессиям</Button>
        </>}
      </main>
    </Screen>
  );
}
