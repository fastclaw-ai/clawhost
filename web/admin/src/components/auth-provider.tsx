"use client";

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import {
  getStoredToken,
  setToken,
  clearToken,
  verifyToken,
} from "@/lib/api";

interface AuthContextType {
  token: string;
  isAuthed: boolean;
  verifying: boolean;
  login: (token: string) => Promise<boolean>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType>({
  token: "",
  isAuthed: false,
  verifying: true,
  login: async () => false,
  logout: () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTokenState] = useState("");
  const [isAuthed, setIsAuthed] = useState(false);
  const [verifying, setVerifying] = useState(true);

  // Verify stored token on mount
  useEffect(() => {
    const stored = getStoredToken();
    if (!stored) {
      setVerifying(false);
      return;
    }
    setTokenState(stored);
    verifyToken(stored).then((valid) => {
      if (valid) {
        setIsAuthed(true);
      } else {
        clearToken();
        setTokenState("");
      }
      setVerifying(false);
    });
  }, []);

  const login = useCallback(async (t: string): Promise<boolean> => {
    const valid = await verifyToken(t);
    if (valid) {
      setToken(t);
      setTokenState(t);
      setIsAuthed(true);
      return true;
    }
    return false;
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setTokenState("");
    setIsAuthed(false);
  }, []);

  return (
    <AuthContext.Provider
      value={{ token, isAuthed, verifying, login, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
