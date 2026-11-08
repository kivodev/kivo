# R8 rules for the app beyond those its libraries ship.

# Google Cast reads the phone's setup by the class name the manifest gives.
-keep class com.mmcdole.kivo.android.mobile.cast.CastSetup { <init>(); }
