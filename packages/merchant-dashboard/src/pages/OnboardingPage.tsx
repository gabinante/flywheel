import { useState, useEffect, useRef } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { api } from '../lib/api';

// ── Types ──────────────────────────────────────────────

interface OnboardingData {
  merchant: {
    id: string;
    status: string;
    businessName: string;
    contactEmail: string;
    contactPhone: string | null;
    website: string | null;
    ghlLocationId: string | null;
    processingConfigured: boolean;
    achConfigured: boolean;
  };
  application: {
    id: string;
    status: string;
    businessType: string | null;
    businessDescription: string | null;
    ein: string | null;
    averageTicketCents: number | null;
    monthlyVolumeCents: number | null;
    nmiBoardingId: string | null;
    nmiBoardingStatus: string | null;
    boardingSubmittedAt: string | null;
    ownerFirstName: string | null;
    ownerLastName: string | null;
    ownerEmail: string | null;
    ownerPhone: string | null;
    ownerDob: string | null;
    ownerSsnLast4: string | null;
    ownerAddress: string | null;
    ownerCity: string | null;
    ownerState: string | null;
    ownerZip: string | null;
    businessAddress: string | null;
    businessCity: string | null;
    businessState: string | null;
    businessZip: string | null;
    bankName: string | null;
    bankRoutingNumber: string | null;
    bankAccountNumber: string | null;
    bankAccountType: string | null;
  } | null;
  step: string;
}

// ── Constants ──────────────────────────────────────────

const BUSINESS_TYPES = ['Sole Proprietorship', 'LLC', 'Corporation', 'Partnership', 'Non-Profit', 'Other'];

const VOLUME_OPTIONS = [
  { label: 'Under $10,000/mo', value: 1000000 },
  { label: '$10,000 - $50,000/mo', value: 5000000 },
  { label: '$50,000 - $100,000/mo', value: 10000000 },
  { label: '$100,000 - $500,000/mo', value: 50000000 },
  { label: 'Over $500,000/mo', value: 100000000 },
];

const US_STATES = [
  'AL','AK','AZ','AR','CA','CO','CT','DE','FL','GA','HI','ID','IL','IN','IA','KS','KY','LA','ME','MD',
  'MA','MI','MN','MS','MO','MT','NE','NV','NH','NJ','NM','NY','NC','ND','OH','OK','OR','PA','RI','SC',
  'SD','TN','TX','UT','VT','VA','WA','WV','WI','WY','DC',
];

const STEPS = [
  { label: 'Business Info', icon: 'M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4' },
  { label: 'Owner & Bank', icon: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z' },
  { label: 'Review', icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z' },
  { label: 'Complete', icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z' },
];

// ── Reusable Components ────────────────────────────────

function StepIndicator({ current }: { current: number }) {
  return (
    <div className="w-full max-w-xl mx-auto mb-8 sm:mb-10">
      <div className="relative flex items-center justify-between mb-2">
        <div className="absolute left-0 right-0 top-1/2 -translate-y-1/2 h-0.5 bg-gray-200 mx-5" />
        <div
          className="absolute left-0 top-1/2 -translate-y-1/2 h-0.5 bg-primary-500 mx-5 transition-all duration-500 ease-out"
          style={{ width: `calc(${(current / (STEPS.length - 1)) * 100}% - 2.5rem)` }}
        />
        {STEPS.map((step, i) => (
          <div key={step.label} className="relative z-10 flex flex-col items-center">
            <div className={`
              flex items-center justify-center w-9 h-9 sm:w-11 sm:h-11 rounded-full
              transition-all duration-500 ease-out
              ${i < current
                ? 'bg-primary-500 text-white scale-100 shadow-md shadow-primary-500/30'
                : i === current
                  ? 'bg-primary-600 text-white scale-110 shadow-lg shadow-primary-500/40 ring-4 ring-primary-100'
                  : 'bg-white text-gray-400 border-2 border-gray-200 scale-100'
              }
            `}>
              {i < current ? (
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" /></svg>
              ) : (
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d={step.icon} /></svg>
              )}
            </div>
          </div>
        ))}
      </div>
      <div className="flex justify-between">
        {STEPS.map((step, i) => (
          <span key={step.label} className={`
            text-[10px] sm:text-xs text-center w-16 sm:w-20 transition-colors duration-300
            ${i === current ? 'text-primary-700 font-semibold' : i < current ? 'text-primary-500' : 'text-gray-400'}
          `}>{step.label}</span>
        ))}
      </div>
    </div>
  );
}

function SlideTransition({ children, step }: { children: React.ReactNode; step: number }) {
  const [displayStep, setDisplayStep] = useState(step);
  const [animClass, setAnimClass] = useState('translate-x-0 opacity-100');
  const prevStep = useRef(step);

  useEffect(() => {
    if (step !== prevStep.current) {
      const goingForward = step > prevStep.current;
      setAnimClass(goingForward ? '-translate-x-8 opacity-0' : 'translate-x-8 opacity-0');
      const timeout = setTimeout(() => {
        setDisplayStep(step);
        setAnimClass(goingForward ? 'translate-x-8 opacity-0' : '-translate-x-8 opacity-0');
        requestAnimationFrame(() => {
          requestAnimationFrame(() => setAnimClass('translate-x-0 opacity-100'));
        });
      }, 200);
      prevStep.current = step;
      return () => clearTimeout(timeout);
    }
  }, [step]);

  const childArray = Array.isArray(children) ? children : [children];
  return (
    <div className={`transition-all duration-300 ease-out ${animClass}`}>
      {childArray[displayStep] || null}
    </div>
  );
}

function InputField({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <div className="group">
      <label className="block text-sm font-medium text-gray-600 mb-1.5 group-focus-within:text-primary-600 transition-colors">
        {label}{required && <span className="text-red-400 ml-0.5">*</span>}
      </label>
      {children}
    </div>
  );
}

const inputClass = 'w-full rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 focus:bg-white transition-all duration-200 outline-none placeholder:text-gray-400';

function SectionHeader({ icon, title, subtitle }: { icon: string; title: string; subtitle: string }) {
  return (
    <div className="flex items-center gap-3 mb-6">
      <div className="flex items-center justify-center w-9 h-9 rounded-xl bg-primary-50 text-primary-600">
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d={icon} />
        </svg>
      </div>
      <div>
        <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
        <p className="text-xs text-gray-400">{subtitle}</p>
      </div>
    </div>
  );
}

function PrimaryButton({ loading, disabled, children, onClick, type = 'submit' }: {
  loading?: boolean; disabled?: boolean; children: React.ReactNode; onClick?: () => void; type?: 'submit' | 'button';
}) {
  return (
    <button type={type} onClick={onClick} disabled={disabled || loading}
      className="w-full bg-gradient-to-r from-primary-600 to-primary-500 hover:from-primary-700 hover:to-primary-600 text-white font-semibold py-3 rounded-xl text-sm transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed shadow-md shadow-primary-500/25 hover:shadow-lg hover:shadow-primary-500/30 active:scale-[0.98]"
    >
      {loading ? (
        <span className="flex items-center justify-center gap-2">
          <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" /><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" /></svg>
          Saving...
        </span>
      ) : children}
    </button>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" onClick={onClick}
      className="flex items-center justify-center gap-1.5 px-5 py-3 bg-gray-50 hover:bg-gray-100 text-gray-600 font-medium rounded-xl text-sm transition-all duration-200 border border-gray-100"
    >
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M11 17l-5-5m0 0l5-5m-5 5h12" /></svg>
      Back
    </button>
  );
}

function StateSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <select value={value} onChange={e => onChange(e.target.value)} className={inputClass}>
      <option value="">State...</option>
      {US_STATES.map(s => <option key={s} value={s}>{s}</option>)}
    </select>
  );
}

// ── Main Component ─────────────────────────────────────

export function OnboardingPage() {
  const navigate = useNavigate();
  const [currentStep, setCurrentStep] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [mounted, setMounted] = useState(false);
  const [manualMode, setManualMode] = useState(false);

  // Step 1: Business info
  const [businessName, setBusinessName] = useState('');
  const [contactPhone, setContactPhone] = useState('');
  const [website, setWebsite] = useState('');
  const [businessType, setBusinessType] = useState('');
  const [businessDescription, setBusinessDescription] = useState('');
  const [ein, setEin] = useState('');
  const [monthlyVolumeCents, setMonthlyVolumeCents] = useState<number | null>(null);
  const [businessAddress, setBusinessAddress] = useState('');
  const [businessCity, setBusinessCity] = useState('');
  const [businessState, setBusinessState] = useState('');
  const [businessZip, setBusinessZip] = useState('');

  // Step 2: Owner & banking
  const [ownerFirstName, setOwnerFirstName] = useState('');
  const [ownerLastName, setOwnerLastName] = useState('');
  const [ownerEmail, setOwnerEmail] = useState('');
  const [ownerPhone, setOwnerPhone] = useState('');
  const [ownerDob, setOwnerDob] = useState('');
  const [ownerSsnLast4, setOwnerSsnLast4] = useState('');
  const [ownerAddress, setOwnerAddress] = useState('');
  const [ownerCity, setOwnerCity] = useState('');
  const [ownerState, setOwnerState] = useState('');
  const [ownerZip, setOwnerZip] = useState('');
  const [bankName, setBankName] = useState('');
  const [bankRoutingNumber, setBankRoutingNumber] = useState('');
  const [bankAccountNumber, setBankAccountNumber] = useState('');
  const [bankAccountType, setBankAccountType] = useState('checking');

  // Manual fallback
  const [nmiSecurityKey, setNmiSecurityKey] = useState('');
  const [nmiTokenizationKey, setNmiTokenizationKey] = useState('');
  const [seamlesschexApiKey, setSeamlesschexApiKey] = useState('');

  // Boarding status
  const [boardingStatus, setBoardingStatus] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['onboarding'],
    queryFn: () => api.get<OnboardingData>('/onboarding'),
  });

  useEffect(() => { requestAnimationFrame(() => setMounted(true)); }, []);

  // Pre-fill from existing data
  useEffect(() => {
    if (!data) return;
    const m = data.merchant;
    const a = data.application;
    setBusinessName(m.businessName || '');
    setContactPhone(m.contactPhone || '');
    setWebsite(m.website || '');
    if (a) {
      setBusinessType(a.businessType || '');
      setBusinessDescription(a.businessDescription || '');
      setEin(a.ein || '');
      setMonthlyVolumeCents(a.monthlyVolumeCents);
      setBusinessAddress(a.businessAddress || '');
      setBusinessCity(a.businessCity || '');
      setBusinessState(a.businessState || '');
      setBusinessZip(a.businessZip || '');
      setOwnerFirstName(a.ownerFirstName || '');
      setOwnerLastName(a.ownerLastName || '');
      setOwnerEmail(a.ownerEmail || '');
      setOwnerPhone(a.ownerPhone || '');
      setOwnerDob(a.ownerDob || '');
      setOwnerSsnLast4(a.ownerSsnLast4 || '');
      setOwnerAddress(a.ownerAddress || '');
      setOwnerCity(a.ownerCity || '');
      setOwnerState(a.ownerState || '');
      setOwnerZip(a.ownerZip || '');
      setBankName(a.bankName || '');
      setBankRoutingNumber(a.bankRoutingNumber || '');
      setBankAccountNumber(a.bankAccountNumber || '');
      setBankAccountType(a.bankAccountType || 'checking');
      if (a.nmiBoardingStatus) setBoardingStatus(a.nmiBoardingStatus);
    }

    // Resume at correct step
    const stepMap: Record<string, number> = {
      business_info: 0, owner_banking: 1, review: 2,
      pending_approval: 3, complete: 3, declined: 2,
    };
    setCurrentStep(stepMap[data.step] ?? 0);
  }, [data]);

  // Poll boarding status when pending
  useEffect(() => {
    if (boardingStatus !== 'PENDING') return;
    const interval = setInterval(async () => {
      try {
        const res = await api.get<{ status: string }>('/onboarding/boarding-status');
        setBoardingStatus(res.status);
        if (res.status === 'APPROVED') {
          setCurrentStep(3);
        }
      } catch { /* ignore */ }
    }, 10000);
    return () => clearInterval(interval);
  }, [boardingStatus]);

  // ── Mutations ──

  const businessMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.patch('/onboarding/business', body),
    onSuccess: () => { setError(null); setCurrentStep(1); },
    onError: (err: Error) => setError(err.message),
  });

  const ownerBankMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.patch('/onboarding/owner-banking', body),
    onSuccess: () => { setError(null); setCurrentStep(2); },
    onError: (err: Error) => setError(err.message),
  });

  const boardingMutation = useMutation({
    mutationFn: () => api.post<{ status: string; activated?: boolean }>('/onboarding/submit-boarding', {}),
    onSuccess: (res) => {
      setError(null);
      if (res.activated) {
        setBoardingStatus('APPROVED');
        setCurrentStep(3);
      } else {
        setBoardingStatus('PENDING');
        setCurrentStep(3);
      }
    },
    onError: (err: Error) => setError(err.message),
  });

  const credentialsMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.post('/onboarding/credentials', body),
    onSuccess: () => { setError(null); setBoardingStatus('APPROVED'); setCurrentStep(3); },
    onError: (err: Error) => setError(err.message),
  });

  // ── Handlers ──

  const handleBusinessSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    businessMutation.mutate({
      businessName, contactPhone: contactPhone || undefined, website: website || undefined,
      businessType, businessDescription: businessDescription || undefined,
      ein: ein || undefined, monthlyVolumeCents: monthlyVolumeCents || undefined,
      businessAddress, businessCity, businessState, businessZip,
    });
  };

  const handleOwnerBankSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    ownerBankMutation.mutate({
      ownerFirstName, ownerLastName, ownerEmail,
      ownerPhone: ownerPhone || undefined, ownerDob, ownerSsnLast4,
      ownerAddress, ownerCity, ownerState, ownerZip,
      bankName, bankRoutingNumber, bankAccountNumber, bankAccountType,
    });
  };

  const handleManualSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!nmiSecurityKey || !nmiTokenizationKey) {
      setError('Gateway Security Key and Tokenization Key are required.');
      return;
    }
    credentialsMutation.mutate({ nmiSecurityKey, nmiTokenizationKey, seamlesschexApiKey: seamlesschexApiKey || undefined });
  };

  // ── Loading ──

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen bg-gradient-to-br from-gray-50 to-blue-50">
        <div className="flex flex-col items-center gap-3">
          <div className="relative">
            <div className="w-12 h-12 rounded-full border-4 border-primary-100" />
            <div className="absolute inset-0 w-12 h-12 rounded-full border-4 border-transparent border-t-primary-500 animate-spin" />
          </div>
          <span className="text-sm text-gray-400">Loading...</span>
        </div>
      </div>
    );
  }

  // ── Render ──

  return (
    <div className={`min-h-screen bg-gradient-to-br from-gray-50 via-white to-blue-50 transition-opacity duration-700 ${mounted ? 'opacity-100' : 'opacity-0'}`}>
      <div className="flex flex-col items-center px-4 sm:px-6 py-8 sm:py-12 max-w-3xl mx-auto w-full">
        {/* Header */}
        <div className={`text-center mb-6 sm:mb-8 transition-all duration-700 delay-100 ${mounted ? 'translate-y-0 opacity-100' : '-translate-y-4 opacity-0'}`}>
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-gradient-to-br from-primary-500 to-primary-700 shadow-lg shadow-primary-500/25 mb-4">
            <svg className="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          <h1 className="text-2xl sm:text-3xl font-bold text-gray-900 tracking-tight">Set Up Your Account</h1>
          <p className="text-gray-500 mt-2 text-sm sm:text-base">We'll get you accepting payments in minutes</p>
        </div>

        {/* Step indicator */}
        <div className={`w-full transition-all duration-700 delay-200 ${mounted ? 'translate-y-0 opacity-100' : '-translate-y-4 opacity-0'}`}>
          <StepIndicator current={currentStep} />
        </div>

        {/* Error */}
        {error && (
          <div className="w-full mb-4 p-4 bg-red-50 border border-red-100 rounded-2xl flex items-start gap-3 animate-[slideDown_0.3s_ease-out]">
            <svg className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            <div className="flex-1">
              <p className="text-sm font-medium text-red-800">Something went wrong</p>
              <p className="text-sm text-red-600 mt-0.5">{error}</p>
            </div>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-600"><svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg></button>
          </div>
        )}

        {/* Steps */}
        <div className={`w-full transition-all duration-700 delay-300 ${mounted ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0'}`}>
          <SlideTransition step={currentStep}>

            {/* ── Step 1: Business Info ── */}
            <form onSubmit={handleBusinessSubmit} className="w-full">
              <div className="bg-white rounded-2xl shadow-sm shadow-gray-200/50 border border-gray-100 p-5 sm:p-7">
                <SectionHeader icon={STEPS[0].icon} title="Business Information" subtitle="Tell us about your business" />
                <div className="space-y-5">
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                    <InputField label="Business Name" required>
                      <input type="text" value={businessName} onChange={e => setBusinessName(e.target.value)} required className={inputClass} placeholder="Your Business Name" />
                    </InputField>
                    <InputField label="Business Type" required>
                      <select value={businessType} onChange={e => setBusinessType(e.target.value)} required className={inputClass}>
                        <option value="">Select type...</option>
                        {BUSINESS_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
                      </select>
                    </InputField>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-5">
                    <InputField label="EIN / Tax ID">
                      <input type="text" value={ein} onChange={e => setEin(e.target.value)} className={inputClass} placeholder="XX-XXXXXXX" />
                    </InputField>
                    <InputField label="Phone">
                      <input type="tel" value={contactPhone} onChange={e => setContactPhone(e.target.value)} className={inputClass} placeholder="(555) 123-4567" />
                    </InputField>
                    <InputField label="Monthly Volume">
                      <select value={monthlyVolumeCents ?? ''} onChange={e => setMonthlyVolumeCents(e.target.value ? Number(e.target.value) : null)} className={inputClass}>
                        <option value="">Select range...</option>
                        {VOLUME_OPTIONS.map(v => <option key={v.value} value={v.value}>{v.label}</option>)}
                      </select>
                    </InputField>
                  </div>
                  <InputField label="Website">
                    <input type="url" value={website} onChange={e => setWebsite(e.target.value)} className={inputClass} placeholder="https://example.com" />
                  </InputField>

                  <div className="relative"><div className="absolute inset-0 flex items-center"><div className="w-full border-t border-gray-100" /></div><div className="relative flex justify-center"><span className="bg-white px-3 text-xs text-gray-400">Business Address</span></div></div>

                  <InputField label="Street Address" required>
                    <input type="text" value={businessAddress} onChange={e => setBusinessAddress(e.target.value)} required className={inputClass} placeholder="123 Main St" />
                  </InputField>
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                    <div className="col-span-2 sm:col-span-2">
                      <InputField label="City" required>
                        <input type="text" value={businessCity} onChange={e => setBusinessCity(e.target.value)} required className={inputClass} placeholder="City" />
                      </InputField>
                    </div>
                    <InputField label="State" required>
                      <StateSelect value={businessState} onChange={setBusinessState} />
                    </InputField>
                    <InputField label="ZIP" required>
                      <input type="text" value={businessZip} onChange={e => setBusinessZip(e.target.value)} required className={inputClass} placeholder="12345" maxLength={10} />
                    </InputField>
                  </div>
                  <InputField label="Business Description">
                    <textarea value={businessDescription} onChange={e => setBusinessDescription(e.target.value)} rows={2} className={`${inputClass} resize-none`} placeholder="What does your business do?" />
                  </InputField>
                </div>
                <div className="mt-7">
                  <PrimaryButton loading={businessMutation.isPending} disabled={!businessName || !businessType || !businessAddress || !businessCity || !businessState || !businessZip}>
                    <span className="flex items-center justify-center gap-2">Continue <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg></span>
                  </PrimaryButton>
                </div>
              </div>
            </form>

            {/* ── Step 2: Owner & Banking ── */}
            <form onSubmit={handleOwnerBankSubmit} className="w-full">
              <div className="bg-white rounded-2xl shadow-sm shadow-gray-200/50 border border-gray-100 p-5 sm:p-7">
                <SectionHeader icon={STEPS[1].icon} title="Owner & Banking" subtitle="Required for payment processing setup" />

                <h3 className="text-sm font-semibold text-gray-700 mb-3">Principal Owner</h3>
                <div className="space-y-5 mb-6">
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                    <InputField label="First Name" required>
                      <input type="text" value={ownerFirstName} onChange={e => setOwnerFirstName(e.target.value)} required className={inputClass} placeholder="First name" />
                    </InputField>
                    <InputField label="Last Name" required>
                      <input type="text" value={ownerLastName} onChange={e => setOwnerLastName(e.target.value)} required className={inputClass} placeholder="Last name" />
                    </InputField>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                    <InputField label="Email" required>
                      <input type="email" value={ownerEmail} onChange={e => setOwnerEmail(e.target.value)} required className={inputClass} placeholder="owner@business.com" />
                    </InputField>
                    <InputField label="Phone">
                      <input type="tel" value={ownerPhone} onChange={e => setOwnerPhone(e.target.value)} className={inputClass} placeholder="(555) 123-4567" />
                    </InputField>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                    <InputField label="Date of Birth" required>
                      <input type="date" value={ownerDob} onChange={e => setOwnerDob(e.target.value)} required className={inputClass} />
                    </InputField>
                    <InputField label="SSN (Last 4)" required>
                      <input type="password" value={ownerSsnLast4} onChange={e => setOwnerSsnLast4(e.target.value.replace(/\D/g, '').slice(0, 4))} required className={inputClass} placeholder="••••" maxLength={4} />
                    </InputField>
                  </div>
                  <InputField label="Home Address" required>
                    <input type="text" value={ownerAddress} onChange={e => setOwnerAddress(e.target.value)} required className={inputClass} placeholder="123 Main St" />
                  </InputField>
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                    <div className="col-span-2 sm:col-span-2">
                      <InputField label="City" required><input type="text" value={ownerCity} onChange={e => setOwnerCity(e.target.value)} required className={inputClass} placeholder="City" /></InputField>
                    </div>
                    <InputField label="State" required><StateSelect value={ownerState} onChange={setOwnerState} /></InputField>
                    <InputField label="ZIP" required><input type="text" value={ownerZip} onChange={e => setOwnerZip(e.target.value)} required className={inputClass} placeholder="12345" maxLength={10} /></InputField>
                  </div>
                </div>

                <div className="relative mb-6"><div className="absolute inset-0 flex items-center"><div className="w-full border-t border-gray-100" /></div><div className="relative flex justify-center"><span className="bg-white px-3 text-xs text-gray-400">Settlement Bank Account</span></div></div>

                <div className="space-y-5">
                  <InputField label="Bank Name" required>
                    <input type="text" value={bankName} onChange={e => setBankName(e.target.value)} required className={inputClass} placeholder="Bank of America, Chase, etc." />
                  </InputField>
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-5">
                    <InputField label="Routing Number" required>
                      <input type="text" value={bankRoutingNumber} onChange={e => setBankRoutingNumber(e.target.value.replace(/\D/g, '').slice(0, 9))} required className={inputClass} placeholder="9 digits" maxLength={9} />
                    </InputField>
                    <InputField label="Account Number" required>
                      <input type="password" value={bankAccountNumber} onChange={e => setBankAccountNumber(e.target.value.replace(/\D/g, ''))} required className={inputClass} placeholder="Account number" />
                    </InputField>
                    <InputField label="Account Type" required>
                      <select value={bankAccountType} onChange={e => setBankAccountType(e.target.value)} className={inputClass}>
                        <option value="checking">Checking</option>
                        <option value="savings">Savings</option>
                      </select>
                    </InputField>
                  </div>
                </div>

                <div className="flex gap-3 mt-7">
                  <BackButton onClick={() => { setError(null); setCurrentStep(0); }} />
                  <PrimaryButton loading={ownerBankMutation.isPending} disabled={!ownerFirstName || !ownerLastName || !ownerEmail || !ownerDob || !ownerSsnLast4 || !ownerAddress || !ownerCity || !ownerState || !ownerZip || !bankName || !bankRoutingNumber || !bankAccountNumber}>
                    <span className="flex items-center justify-center gap-2">Continue <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg></span>
                  </PrimaryButton>
                </div>
              </div>

              <div className="mt-4 p-4 bg-blue-50/50 border border-blue-100 rounded-2xl flex items-start gap-3">
                <svg className="w-5 h-5 text-blue-400 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>
                <p className="text-xs text-blue-600/80 leading-relaxed">Your data is encrypted and transmitted securely. Bank details are sent directly to our payment processor for underwriting — we don't store full account numbers.</p>
              </div>
            </form>

            {/* ── Step 3: Review & Submit ── */}
            <div className="w-full">
              <div className="bg-white rounded-2xl shadow-sm shadow-gray-200/50 border border-gray-100 p-5 sm:p-7">
                <SectionHeader icon={STEPS[2].icon} title="Review & Submit" subtitle="Confirm your details" />

                {/* Summary cards */}
                <div className="space-y-4 mb-6">
                  <div className="bg-gray-50 rounded-xl p-4">
                    <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Business</h4>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
                      <span className="text-gray-500">Name</span><span className="font-medium text-gray-900">{businessName}</span>
                      <span className="text-gray-500">Type</span><span>{businessType}</span>
                      {ein && <><span className="text-gray-500">EIN</span><span>{ein}</span></>}
                      <span className="text-gray-500">Address</span><span>{businessAddress}, {businessCity}, {businessState} {businessZip}</span>
                    </div>
                  </div>
                  <div className="bg-gray-50 rounded-xl p-4">
                    <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Owner</h4>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
                      <span className="text-gray-500">Name</span><span className="font-medium text-gray-900">{ownerFirstName} {ownerLastName}</span>
                      <span className="text-gray-500">Email</span><span>{ownerEmail}</span>
                      <span className="text-gray-500">Address</span><span>{ownerAddress}, {ownerCity}, {ownerState} {ownerZip}</span>
                    </div>
                  </div>
                  <div className="bg-gray-50 rounded-xl p-4">
                    <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Bank Account</h4>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
                      <span className="text-gray-500">Bank</span><span className="font-medium text-gray-900">{bankName}</span>
                      <span className="text-gray-500">Routing</span><span>••••{bankRoutingNumber.slice(-4)}</span>
                      <span className="text-gray-500">Account</span><span>••••{bankAccountNumber.slice(-4)}</span>
                      <span className="text-gray-500">Type</span><span className="capitalize">{bankAccountType}</span>
                    </div>
                  </div>
                </div>

                {/* Mode toggle */}
                <div className="bg-gray-50 rounded-xl p-1 flex mb-6">
                  <button type="button" onClick={() => setManualMode(false)}
                    className={`flex-1 py-2.5 rounded-lg text-sm font-medium transition-all duration-200 ${!manualMode ? 'bg-white shadow-sm text-primary-700' : 'text-gray-500 hover:text-gray-700'}`}>
                    Submit for Approval
                  </button>
                  <button type="button" onClick={() => setManualMode(true)}
                    className={`flex-1 py-2.5 rounded-lg text-sm font-medium transition-all duration-200 ${manualMode ? 'bg-white shadow-sm text-primary-700' : 'text-gray-500 hover:text-gray-700'}`}>
                    I have my own keys
                  </button>
                </div>

                {!manualMode ? (
                  <div>
                    <p className="text-sm text-gray-500 mb-4">We'll submit your application for underwriting. Most merchants are approved within minutes.</p>
                    <div className="flex gap-3">
                      <BackButton onClick={() => { setError(null); setCurrentStep(1); }} />
                      <PrimaryButton type="button" loading={boardingMutation.isPending} onClick={() => boardingMutation.mutate()}>
                        <span className="flex items-center justify-center gap-2">
                          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
                          Submit Application
                        </span>
                      </PrimaryButton>
                    </div>
                  </div>
                ) : (
                  <form onSubmit={handleManualSubmit}>
                    <p className="text-sm text-gray-500 mb-4">Already have your payment gateway credentials? Enter them below to activate immediately.</p>
                    <div className="space-y-4 mb-6">
                      <InputField label="Gateway Security Key" required>
                        <input type="password" value={nmiSecurityKey} onChange={e => setNmiSecurityKey(e.target.value)} required className={`${inputClass} font-mono`} placeholder="From your NMI dashboard" />
                      </InputField>
                      <InputField label="Tokenization Key" required>
                        <input type="text" value={nmiTokenizationKey} onChange={e => setNmiTokenizationKey(e.target.value)} required className={`${inputClass} font-mono`} placeholder="Collect.js public key" />
                      </InputField>
                      <InputField label="Seamlesschex API Key">
                        <input type="password" value={seamlesschexApiKey} onChange={e => setSeamlesschexApiKey(e.target.value)} className={`${inputClass} font-mono`} placeholder="Optional — for ACH payments" />
                      </InputField>
                    </div>
                    <div className="flex gap-3">
                      <BackButton onClick={() => { setError(null); setCurrentStep(1); }} />
                      <PrimaryButton loading={credentialsMutation.isPending} disabled={!nmiSecurityKey || !nmiTokenizationKey}>
                        <span className="flex items-center justify-center gap-2">Connect & Activate</span>
                      </PrimaryButton>
                    </div>
                  </form>
                )}
              </div>
            </div>

            {/* ── Step 4: Pending / Complete ── */}
            <div className="w-full">
              {boardingStatus === 'PENDING' ? (
                <div className="bg-white rounded-2xl shadow-sm shadow-gray-200/50 border border-gray-100 p-7 sm:p-10 text-center">
                  <div className="relative inline-flex mb-6">
                    <div className="w-20 h-20 rounded-full bg-primary-50 flex items-center justify-center">
                      <svg className="w-10 h-10 text-primary-500 animate-pulse" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                  </div>
                  <h2 className="text-xl sm:text-2xl font-bold text-gray-900 mb-2">Application Submitted</h2>
                  <p className="text-gray-500 text-sm sm:text-base max-w-sm mx-auto mb-4">
                    Your merchant application is being reviewed. Most applications are approved within minutes.
                  </p>
                  <div className="inline-flex items-center gap-2 bg-primary-50 text-primary-700 text-sm font-medium px-4 py-2 rounded-full">
                    <div className="w-2 h-2 bg-primary-500 rounded-full animate-pulse" />
                    Checking status...
                  </div>
                </div>
              ) : (
                <div className="bg-white rounded-2xl shadow-sm shadow-gray-200/50 border border-gray-100 p-7 sm:p-10 text-center">
                  <div className="relative inline-flex mb-6">
                    <div className="w-20 h-20 bg-green-50 rounded-full flex items-center justify-center animate-[scaleIn_0.5s_ease-out]">
                      <svg className="w-10 h-10 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                    <div className="absolute -top-1 -right-1 w-6 h-6 bg-green-400 rounded-full flex items-center justify-center animate-[bounceIn_0.6s_ease-out_0.3s_both]">
                      <svg className="w-3.5 h-3.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}><path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" /></svg>
                    </div>
                  </div>
                  <h2 className="text-xl sm:text-2xl font-bold text-gray-900 mb-2">You're All Set!</h2>
                  <p className="text-gray-500 text-sm sm:text-base max-w-sm mx-auto mb-8">
                    Your payment processing is configured and your account is active. Start accepting payments now.
                  </p>
                  <button onClick={() => navigate('/')}
                    className="w-full sm:w-auto sm:px-12 bg-gradient-to-r from-primary-600 to-primary-500 hover:from-primary-700 hover:to-primary-600 text-white font-semibold py-3 rounded-xl text-sm transition-all duration-200 shadow-md shadow-primary-500/25 hover:shadow-lg hover:shadow-primary-500/30 active:scale-[0.98]"
                  >
                    <span className="flex items-center justify-center gap-2">Go to Dashboard <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg></span>
                  </button>
                </div>
              )}
            </div>

          </SlideTransition>
        </div>

        <p className="text-xs text-gray-300 mt-8">GoHighPayment &middot; Secure payment processing</p>
      </div>

      <style>{`
        @keyframes scaleIn { from { transform: scale(0); opacity: 0; } to { transform: scale(1); opacity: 1; } }
        @keyframes bounceIn { 0% { transform: scale(0); opacity: 0; } 60% { transform: scale(1.2); } 100% { transform: scale(1); opacity: 1; } }
        @keyframes slideDown { from { transform: translateY(-8px); opacity: 0; } to { transform: translateY(0); opacity: 1; } }
      `}</style>
    </div>
  );
}
