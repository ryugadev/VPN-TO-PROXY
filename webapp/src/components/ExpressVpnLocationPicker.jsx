import React, { useState, useEffect } from 'react';
import { Search, Star, Globe, Compass, ChevronDown, ChevronUp, Check } from 'lucide-react';

const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

export default function ExpressVpnLocationPicker({ onSelect, selectedAlias }) {
  const [regions, setRegions] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [activeTab, setActiveTab] = useState('recommended'); // 'recommended', 'all', 'favorites'
  const [favorites, setFavorites] = useState(() => {
    try {
      const saved = localStorage.getItem('expressvpn_favorites');
      return saved ? JSON.parse(saved) : [];
    } catch {
      return [];
    }
  });
  const [expandedRegions, setExpandedRegions] = useState({});

  useEffect(() => {
    // Fetch locations from backend API
    // Dynamic query: checks if we are running in mock mode by looking at query params or defaults to expressvpn_mock
    fetch(`${API_BASE}/vpn/expressvpn/locations?type=expressvpn_mock`)
      .then(res => res.json())
      .then(data => {
        if (data && data.regions) {
          setRegions(data.regions);
          // Expand first region by default
          if (data.regions.length > 0) {
            setExpandedRegions({ [data.regions[0].name]: true });
          }
        }
      })
      .catch(err => console.error('Failed to load locations', err));
  }, []);

  const toggleFavorite = (alias, e) => {
    e.stopPropagation();
    const nextFavorites = favorites.includes(alias)
      ? favorites.filter(f => f !== alias)
      : [...favorites, alias];
    setFavorites(nextFavorites);
    localStorage.setItem('expressvpn_favorites', JSON.stringify(nextFavorites));
  };

  const toggleRegion = (regionName) => {
    setExpandedRegions(prev => ({
      ...prev,
      [regionName]: !prev[regionName]
    }));
  };

  // Flatten locations for search and tabs
  const allLocations = regions.flatMap(r => r.locations || []);

  const filteredLocations = allLocations.filter(loc => {
    const term = searchTerm.toLowerCase();
    return (
      loc.country.toLowerCase().includes(term) ||
      loc.city.toLowerCase().includes(term) ||
      loc.displayName.toLowerCase().includes(term) ||
      loc.alias.toLowerCase().includes(term) ||
      loc.region.toLowerCase().includes(term)
    );
  });

  const getRecommendedLocations = () => {
    return filteredLocations.filter(loc => loc.recommended);
  };

  const getFavoriteLocations = () => {
    return filteredLocations.filter(loc => favorites.includes(loc.alias));
  };

  const renderLocationItem = (loc) => {
    const isSelected = selectedAlias === loc.alias;
    const isFav = favorites.includes(loc.alias);

    return (
      <div
        key={loc.alias}
        onClick={() => onSelect(loc)}
        className={`flex items-center justify-between p-3 rounded-xl cursor-pointer transition-all duration-200 border ${
          isSelected
            ? 'bg-purple-500/20 border-purple-500/50 shadow-md shadow-purple-500/10'
            : 'bg-slate-800/40 border-slate-700/50 hover:bg-slate-800/80 hover:border-slate-600'
        }`}
      >
        <div className="flex items-center gap-3">
          <span className="text-2xl select-none">{loc.flag || '🌐'}</span>
          <div>
            <div className="text-sm font-semibold text-slate-100">{loc.country}</div>
            {loc.city && loc.city !== loc.country && (
              <div className="text-xs text-slate-400">{loc.city}</div>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={(e) => toggleFavorite(loc.alias, e)}
            className={`p-1.5 rounded-lg hover:bg-slate-700/50 transition-colors ${
              isFav ? 'text-amber-400' : 'text-slate-500 hover:text-slate-300'
            }`}
          >
            <Star className="w-4 h-4 fill-current" />
          </button>
          {isSelected && (
            <div className="w-5 h-5 bg-purple-500 text-white rounded-full flex items-center justify-center">
              <Check className="w-3.5 h-3.5" />
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="flex flex-col gap-4 bg-slate-900/40 border border-slate-800/80 p-4 rounded-2xl">
      {/* Search Bar */}
      <div className="relative">
        <Search className="absolute left-3 top-3.5 w-4 h-4 text-slate-400" />
        <input
          type="text"
          placeholder="Search for a city or country"
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          className="w-full pl-9 pr-4 py-2.5 bg-slate-950/60 border border-slate-800 focus:border-purple-500/60 focus:ring-1 focus:ring-purple-500/60 rounded-xl text-sm text-slate-100 placeholder-slate-500 transition-all duration-300 outline-none"
        />
      </div>

      {/* Tabs */}
      <div className="flex gap-1.5 p-1 bg-slate-950/40 border border-slate-800/50 rounded-xl">
        <button
          type="button"
          onClick={() => setActiveTab('recommended')}
          className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-lg transition-all ${
            activeTab === 'recommended'
              ? 'bg-slate-800 text-purple-400 shadow-sm'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Compass className="w-3.5 h-3.5" />
          Recommended
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('all')}
          className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-lg transition-all ${
            activeTab === 'all'
              ? 'bg-slate-800 text-purple-400 shadow-sm'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Globe className="w-3.5 h-3.5" />
          All Locations
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('favorites')}
          className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-lg transition-all ${
            activeTab === 'favorites'
              ? 'bg-slate-800 text-purple-400 shadow-sm'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Star className="w-3.5 h-3.5" />
          Favorites ({favorites.length})
        </button>
      </div>

      {/* Locations Display */}
      <div className="max-h-[260px] overflow-y-auto pr-1 flex flex-col gap-2 scrollbar-thin">
        {activeTab === 'recommended' && (
          <div className="flex flex-col gap-2">
            {getRecommendedLocations().length > 0 ? (
              getRecommendedLocations().map(renderLocationItem)
            ) : (
              <div className="text-center text-xs text-slate-500 py-6">No recommended locations found</div>
            )}
          </div>
        )}

        {activeTab === 'favorites' && (
          <div className="flex flex-col gap-2">
            {getFavoriteLocations().length > 0 ? (
              getFavoriteLocations().map(renderLocationItem)
            ) : (
              <div className="text-center text-xs text-slate-500 py-6">No favorite locations saved yet</div>
            )}
          </div>
        )}

        {activeTab === 'all' && (
          <div className="flex flex-col gap-3">
            {searchTerm ? (
              // Flat view when searching
              filteredLocations.length > 0 ? (
                filteredLocations.map(renderLocationItem)
              ) : (
                <div className="text-center text-xs text-slate-500 py-6">No locations match search</div>
              )
            ) : (
              // Accordion view by region when not searching
              regions.map(region => {
                const isExpanded = expandedRegions[region.name];
                return (
                  <div key={region.name} className="flex flex-col gap-1.5">
                    <button
                      type="button"
                      onClick={() => toggleRegion(region.name)}
                      className="flex items-center justify-between px-2 py-1 text-xs font-semibold text-slate-400 hover:text-slate-200 transition-colors uppercase tracking-wider"
                    >
                      <span>{region.name} ({region.locations?.length || 0})</span>
                      {isExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                    </button>

                    {isExpanded && (
                      <div className="flex flex-col gap-2 pl-1.5 border-l border-slate-800 ml-1.5 py-1">
                        {region.locations?.map(renderLocationItem)}
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
}
