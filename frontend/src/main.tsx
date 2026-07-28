import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";

class AppErrorBoundary extends React.Component<React.PropsWithChildren, { error: string }> {
  state = { error: "" };

  static getDerivedStateFromError(error: unknown) {
    return { error: error instanceof Error ? error.message : String(error) };
  }

  componentDidCatch(error: unknown) {
    console.error("Codex Skill Manager frontend error", error);
  }

  render() {
    if (this.state.error) {
      return (
        <main style={{ padding: 32, fontFamily: "Segoe UI, sans-serif", color: "#172033" }}>
          <h1>界面加载失败</h1>
          <p>请重新启动应用；如果问题持续存在，请将下面的信息用于诊断：</p>
          <pre style={{ whiteSpace: "pre-wrap", padding: 16, background: "#eef2f8", borderRadius: 10 }}>
            {this.state.error}
          </pre>
        </main>
      );
    }
    return this.props.children;
  }
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary><App /></AppErrorBoundary>
  </React.StrictMode>
);
