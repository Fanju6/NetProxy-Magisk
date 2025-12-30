/*
 * This file is auto-generated.  DO NOT MODIFY.
 */
package www.netproxy.web.ui;

public interface IKsuWebuiStandaloneInterface extends android.os.IInterface {
    
    /** Default implementation for IKsuWebuiStandaloneInterface. */
    public static class Default implements IKsuWebuiStandaloneInterface {
        @Override
        public rikka.parcelablelist.ParcelableListSlice<android.content.pm.PackageInfo> getPackages(int flags) throws android.os.RemoteException {
            return null;
        }
        
        @Override
        public android.os.IBinder asBinder() {
            return null;
        }
    }
    
    /** Local-side IPC implementation stub class. */
    public static abstract class Stub extends android.os.Binder implements IKsuWebuiStandaloneInterface {
        private static final String DESCRIPTOR = "www.netproxy.web.ui.IKsuWebuiStandaloneInterface";
        static final int TRANSACTION_getPackages = (android.os.IBinder.FIRST_CALL_TRANSACTION + 0);
        
        /** Construct the stub at attach it to the interface. */
        public Stub() {
            this.attachInterface(this, DESCRIPTOR);
        }
        
        /**
         * Cast an IBinder object into an IKsuWebuiStandaloneInterface interface,
         * generating a proxy if needed.
         */
        public static IKsuWebuiStandaloneInterface asInterface(android.os.IBinder obj) {
            if ((obj == null)) {
                return null;
            }
            android.os.IInterface iin = obj.queryLocalInterface(DESCRIPTOR);
            if (((iin != null) && (iin instanceof IKsuWebuiStandaloneInterface))) {
                return ((IKsuWebuiStandaloneInterface) iin);
            }
            return new Stub.Proxy(obj);
        }
        
        @Override
        public android.os.IBinder asBinder() {
            return this;
        }
        
        @Override
        public boolean onTransact(int code, android.os.Parcel data, android.os.Parcel reply, int flags) throws android.os.RemoteException {
            String descriptor = DESCRIPTOR;
            switch (code) {
                case INTERFACE_TRANSACTION: {
                    reply.writeString(descriptor);
                    return true;
                }
                case TRANSACTION_getPackages: {
                    data.enforceInterface(descriptor);
                    int _arg0 = data.readInt();
                    rikka.parcelablelist.ParcelableListSlice<android.content.pm.PackageInfo> _result = this.getPackages(_arg0);
                    reply.writeNoException();
                    if ((_result != null)) {
                        reply.writeInt(1);
                        _result.writeToParcel(reply, android.os.Parcelable.PARCELABLE_WRITE_RETURN_VALUE);
                    } else {
                        reply.writeInt(0);
                    }
                    return true;
                }
                default: {
                    return super.onTransact(code, data, reply, flags);
                }
            }
        }
        
        private static class Proxy implements IKsuWebuiStandaloneInterface {
            private android.os.IBinder mRemote;
            
            Proxy(android.os.IBinder remote) {
                mRemote = remote;
            }
            
            @Override
            public android.os.IBinder asBinder() {
                return mRemote;
            }
            
            public String getInterfaceDescriptor() {
                return DESCRIPTOR;
            }
            
            @Override
            public rikka.parcelablelist.ParcelableListSlice<android.content.pm.PackageInfo> getPackages(int flags) throws android.os.RemoteException {
                android.os.Parcel _data = android.os.Parcel.obtain();
                android.os.Parcel _reply = android.os.Parcel.obtain();
                rikka.parcelablelist.ParcelableListSlice<android.content.pm.PackageInfo> _result;
                try {
                    _data.writeInterfaceToken(DESCRIPTOR);
                    _data.writeInt(flags);
                    mRemote.transact(Stub.TRANSACTION_getPackages, _data, _reply, 0);
                    _reply.readException();
                    if ((0 != _reply.readInt())) {
                        _result = rikka.parcelablelist.ParcelableListSlice.CREATOR.createFromParcel(_reply);
                    } else {
                        _result = null;
                    }
                } finally {
                    _reply.recycle();
                    _data.recycle();
                }
                return _result;
            }
        }
    }
    
    public rikka.parcelablelist.ParcelableListSlice<android.content.pm.PackageInfo> getPackages(int flags) throws android.os.RemoteException;
}
